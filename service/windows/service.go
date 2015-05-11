// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/service/common"
)

var (
	logger   = loggo.GetLogger("juju.worker.deployer.service")
	renderer = &shell.PowershellRenderer{}

	// ERROR_SERVICE_DOES_NOT_EXIST is returned by the OS when trying to open
	// an inexistent service
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms684330%28v=vs.85%29.aspx
	ERROR_SERVICE_DOES_NOT_EXIST syscall.Errno = 0x424

	// ERROR_LOGON_NOT_GRANTED is returned, if the user is not allowed to
	// login as a service
	ERROR_LOGON_NOT_GRANTED syscall.Errno = 0x564

	// ERROR_LOGON_FAILURE is returned when username and/or password is
	// incorrect
	ERROR_LOGON_FAILURE syscall.Errno = 0x52E

	// ERROR_SERVICE_EXISTS is returned by the operating system if the service
	// we are trying to create, already exists
	ERROR_SERVICE_EXISTS syscall.Errno = 0x431

	// The syscall package in go 1.2.1 does not have this error defined. Remove this
	// when we update the go version we use to build juju
	ERROR_MORE_DATA syscall.Errno = 0xEA

	// This is the user under which juju services start. We chose to use a
	// normal user for this purpose because some installers require a normal
	// user with a proper user profile to actually run. This user is created
	// via userdata, and should exist on all juju bootstrapped systems.
	// Required privileges for this user are:
	// SeAssignPrimaryTokenPrivilege
	// SeServiceLogonRight
	jujudUser = ".\\jujud"

	// File containing encrypted password for jujud user.
	// TODO (gabriel-samfira): migrate this to a registry key
	jujuPasswdFile = "C:\\Juju\\Jujud.pass"
)

// Service represents a service running on the current system
type Service struct {
	common.Service
	manager ServiceManagerInterface
}

// ServiceManagerInterface exposes methods needed to manage a windows service
type ServiceManagerInterface interface {
	// Start starts a service.
	Start(name string) error
	// Stop stops a service.
	Stop(name string) error
	// Delete deletes a service.
	Delete(name string) error
	// Create creates a service with the given config.
	Create(name string, conf common.Conf) error
	// Running returns the status of a service.
	Running(name string) (bool, error)
	// Exists checks whether the config of the installed service matches the
	// config supplied to this function
	Exists(name string, conf common.Conf) (bool, error)
}

func newService(name string, conf common.Conf) (*Service, error) {
	m, err := newServiceManager()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Service{
		Service: common.Service{
			Name: name,
			Conf: conf,
		},
		manager: m,
	}, nil
}

// NewService returns a new Service type
func NewService(name string, conf common.Conf) (*Service, error) {
	return newService(name, conf)
}

// IsRunning returns whether or not windows is the local init system.
func IsRunning() (bool, error) {
	return runtime.GOOS == "windows", nil
}

// ListServices returns the name of all installed services on the
// local host.
func ListServices() ([]string, error) {
	return listServices()
}

// ListCommand returns a command that will list the services on a host.
func ListCommand() string {
	return `(Get-Service).Name`
}

// Start starts the service.
func (s *Service) Start() error {
	logger.Infof("Starting service %q", s.Service.Name)
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		logger.Infof("Service %q already running", s.Service.Name)
		return nil
	}
	err = s.manager.Start(s.Name())
	return err
}

// Stop stops the service.
func (s *Service) Stop() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return nil
	}
	err = s.manager.Stop(s.Name())
	return err
}

// Install installs and starts the service.
func (s *Service) Install() error {
	err := s.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
		return errors.New(fmt.Sprintf("Service %s already installed", s.Service.Name))
	}

	logger.Infof("Installing Service %v", s.Name)
	err = s.manager.Create(s.Name(), s.Conf())
	if err != nil {
		return errors.Trace(err)
	}
	return s.Start()
}

// Remove deletes the service.
func (s *Service) Remove() error {
	installed, err := s.Installed()
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}

	err = s.Stop()
	if err != nil {
		return errors.Trace(err)
	}
	err = s.manager.Delete(s.Name())
	return err
}

// Name implements service.Service.
func (s *Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s *Service) Conf() common.Conf {
	return s.Service.Conf
}

func (s *Service) Running() (bool, error) {
	if ok, err := s.Installed(); err != nil {
		return false, errors.Trace(err)
	} else {
		if !ok {
			return false, nil
		}
	}
	return s.manager.Running(s.Name())
}

// Exists returns whether the service configuration reflects the
// desired state
func (s *Service) Exists() (bool, error) {
	return s.manager.Exists(s.Name(), s.Conf())
}

// Installed returns whether the service is installed
func (s *Service) Installed() (bool, error) {
	services, err := ListServices()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, val := range services {
		if s.Name() == val {
			return true, nil
		}
	}
	return false, nil
}

// Validate checks the service for invalid values.
func (s *Service) Validate() error {
	if err := s.Service.Validate(renderer); err != nil {
		return errors.Trace(err)
	}

	if s.Service.Conf.Transient {
		return errors.NotSupportedf("transient services")
	}

	if s.Service.Conf.AfterStopped != "" {
		return errors.NotSupportedf("Conf.AfterStopped")
	}

	return nil
}

// InstallCommands returns shell commands to install the service.
func (s *Service) InstallCommands() ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallCommands[1:],
		renderer.Quote(s.Service.Name),
		renderer.Quote(s.Service.Conf.Desc),
		renderer.Quote(s.Service.Conf.ExecStart),
	)
	return strings.Split(cmd, "\n"), nil
}

// StartCommands returns shell commands to start the service.
func (s *Service) StartCommands() ([]string, error) {
	cmd := fmt.Sprintf(`Start-Service %s`, renderer.Quote(s.Service.Name))
	return []string{cmd}, nil
}

const serviceInstallCommands = `
New-Service -Credential $jujuCreds -Name %s -DependsOn Winmgmt -DisplayName %s %s`
