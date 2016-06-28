// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
)

var (
	NewPingTimeout               = newPingTimeout
	MaxClientPingInterval        = &maxClientPingInterval
	MongoPingInterval            = &mongoPingInterval
	NewBackups                   = &newBackups
	AllowedMethodsDuringUpgrades = allowedMethodsDuringUpgrades
	BZMimeType                   = bzMimeType
	JSMimeType                   = jsMimeType
	SpritePath                   = spritePath
)

func ServerMacaroon(srv *Server) (*macaroon.Macaroon, error) {
	auth, err := srv.authCtxt.macaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Macaroon, nil
}

func ServerBakeryService(srv *Server) (authentication.BakeryService, error) {
	auth, err := srv.authCtxt.macaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Service, nil
}

// ServerAuthenticatorForTag calls the authenticatorForTag method
// of the server's authContext.
func ServerAuthenticatorForTag(srv *Server, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return srv.authCtxt.authenticatorForTag(tag)
}

func ApiHandlerWithEntity(entity state.Entity) *apiHandler {
	return &apiHandler{entity: entity}
}

const LoginRateLimit = loginRateLimit

// DelayLogins changes how the Login code works so that logins won't proceed
// until they get a message on the returned channel.
// After calling this function, the caller is responsible for sending messages
// on the nextChan in order for Logins to succeed. The original behavior can be
// restored by calling the cleanup function.
func DelayLogins() (nextChan chan struct{}, cleanup func()) {
	nextChan = make(chan struct{}, 10)
	cleanup = func() {
		doCheckCreds = checkCreds
	}
	delayedCheckCreds := func(st *state.State, c params.LoginRequest, lookForModelUser bool, authenticator authentication.EntityAuthenticator) (state.Entity, *time.Time, error) {
		<-nextChan
		return checkCreds(st, c, lookForModelUser, authenticator)
	}
	doCheckCreds = delayedCheckCreds
	return
}

func NewErrRoot(err error) *errRoot {
	return &errRoot{err}
}

// TestingApiRoot gives you an ApiRoot as a rpc.Methodfinder that is
// *barely* connected to anything.  Just enough to let you probe some
// of the interfaces, but not enough to actually do any RPC calls.
func TestingApiRoot(st *state.State) rpc.MethodFinder {
	return newApiRoot(st, common.NewResources(), nil)
}

// TestingApiHandler gives you an ApiHandler that isn't connected to
// anything real. It's enough to let test some basic functionality though.
func TestingApiHandler(c *gc.C, srvSt, st *state.State) (*apiHandler, *common.Resources) {
	authCtxt, err := newAuthContext(srvSt)
	c.Assert(err, jc.ErrorIsNil)
	srv := &Server{
		authCtxt: authCtxt,
		state:    srvSt,
		tag:      names.NewMachineTag("0"),
	}
	h, err := newApiHandler(srv, st, nil, nil, st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	return h, h.getResources()
}

// TestingUpgradingRoot returns a limited srvRoot
// in an upgrade scenario.
func TestingUpgradingRoot(st *state.State) rpc.MethodFinder {
	r := TestingApiRoot(st)
	return newUpgradingRoot(r)
}

// TestingRestrictedApiHandler returns a restricted srvRoot as if accessed
// from the root of the API path.
func TestingRestrictedApiHandler(st *state.State) rpc.MethodFinder {
	r := TestingApiRoot(st)
	return newRestrictedRoot(r)
}

type preFacadeAdminApi struct{}

func newPreFacadeAdminApi(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &preFacadeAdminApi{}
}

func (r *preFacadeAdminApi) Admin(id string) (*preFacadeAdminApi, error) {
	return r, nil
}

var PreFacadeModelTag = names.NewModelTag("383c49f3-526d-4f9e-b50a-1e6fa4e9b3d9")

func (r *preFacadeAdminApi) Login(c params.Creds) (params.LoginResult, error) {
	return params.LoginResult{
		ModelTag: PreFacadeModelTag.String(),
	}, nil
}

type failAdminApi struct{}

func newFailAdminApi(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &failAdminApi{}
}

func (r *failAdminApi) Admin(id string) (*failAdminApi, error) {
	return r, nil
}

func (r *failAdminApi) Login(c params.Creds) (params.LoginResult, error) {
	return params.LoginResult{}, fmt.Errorf("fail")
}

// SetPreFacadeAdminApi is used to create a test scenario where the API server
// does not know about API facade versioning. In this case, the client should
// login to the v1 facade, which sends backwards-compatible login fields.
// The v0 facade will fail on a pre-defined error.
func SetPreFacadeAdminApi(srv *Server) {
	srv.adminApiFactories = map[int]adminApiFactory{
		0: newFailAdminApi,
		1: newPreFacadeAdminApi,
	}
}

func SetAdminApiVersions(srv *Server, versions ...int) {
	factories := make(map[int]adminApiFactory)
	for _, n := range versions {
		switch n {
		case 3:
			factories[n] = newAdminApiV3
		default:
			panic(fmt.Errorf("unknown admin API version %d", n))
		}
	}
	srv.adminApiFactories = factories
}

// TestingRestoreInProgressRoot returns a limited restoreInProgressRoot
// containing a srvRoot as returned by TestingSrvRoot.
func TestingRestoreInProgressRoot(st *state.State) *restoreInProgressRoot {
	r := TestingApiRoot(st)
	return newRestoreInProgressRoot(r)
}

// TestingAboutToRestoreRoot returns a limited aboutToRestoreRoot
// containing a srvRoot as returned by TestingSrvRoot.
func TestingAboutToRestoreRoot(st *state.State) *aboutToRestoreRoot {
	r := TestingApiRoot(st)
	return newAboutToRestoreRoot(r)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() *net.TCPAddr {
	return srv.lis.Addr().(*net.TCPAddr) // cannot fail
}

// PatchGetMigrationBackend overrides the getMigrationBackend function
// to support testing.
func PatchGetMigrationBackend(p Patcher, st migrationBackend) {
	p.PatchValue(&getMigrationBackend, func(*state.State) migrationBackend {
		return st
	})
}

// PatchGetControllerCACert overrides the getControllerCACert function
// to support testing.
func PatchGetControllerCACert(p Patcher, caCert string) {
	p.PatchValue(&getControllerCACert, func(migrationBackend) (string, error) {
		return caCert, nil
	})
}

// Patcher defines an interface that matches the PatchValue method on
// CleanupSuite
type Patcher interface {
	PatchValue(ptr, value interface{})
}
