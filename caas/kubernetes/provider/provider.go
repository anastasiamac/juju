// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	apicaasunitprovisioner "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

type kubernetesEnvironProvider struct {
	environProviderCredentials
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)
var providerInstance = kubernetesEnvironProvider{}

// Version is part of the EnvironProvider interface.
func (kubernetesEnvironProvider) Version() int {
	return 0
}

func newK8sClient(c *rest.Config) (kubernetes.Interface, error) {
	client, err := kubernetes.NewForConfig(c)
	return client, err
}

// Open is part of the ContainerEnvironProvider interface.
func (kubernetesEnvironProvider) Open(args environs.OpenParams) (caas.Broker, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	broker, err := NewK8sBroker(args.Cloud, args.Config.Name(), newK8sClient)
	if err != nil {
		return nil, err
	}
	return broker, nil
}

// ParsePodSpec is part of the ContainerEnvironProvider interface.
func (kubernetesEnvironProvider) ParsePodSpec(in string) (*caas.PodSpec, error) {
	spec, err := parseK8sPodSpec(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return spec, spec.Validate()
}

// BuildPodSpec injects additional segments of configuration into podspec.
func (kubernetesEnvironProvider) BuildPodSpec(spec *caas.PodSpec, info *apicaasunitprovisioner.ProvisioningInfo) (*caas.PodSpec, error) {
	// TODO(ycliuhw): currently we assume constraints are all same for containers in same pod.
	// A todo to support different constraints for containers in same pod.
	// And also, should we consolidate normal constraints like cpu/mem etc with device constraints?
	var constraints []caas.Constraint
	for _, c := range info.Devices {
		constraints = append(constraints, caas.Constraint{
			Type:       caas.DeviceType(c.Type),
			Count:      c.Count,
			Attributes: c.Attributes,
		})
	}
	for _, c := range spec.Containers {
		c.Constraints = constraints
	}
	return spec, nil
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p kubernetesEnvironProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p kubernetesEnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p kubernetesEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default storage sources.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = K8s_ProviderType
	}
	if _, ok := args.Config.StorageDefaultFilesystemSource(); !ok {
		attrs[config.StorageDefaultFilesystemSourceKey] = K8s_ProviderType
	}
	return args.Config.Apply(attrs)
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p kubernetesEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func (p kubernetesEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := url.Parse(spec.Endpoint); err != nil {
		return errors.NotValidf("endpoint %q", spec.Endpoint)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
