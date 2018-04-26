package main

import (
	"github.com/humboldt-xie/registry-replication/auth"
	"github.com/vmware/harbor/src/common/utils/registry"
	"net/http"
)

func newRegistryClient(endpoint string, insecure bool, username, password string) (*registry.Registry, error) {
	transport := registry.GetHTTPTransport(insecure)
	credential := auth.NewBasicAuthCredential(username, password)
	authorizer := auth.NewStandardTokenAuthorizer(&http.Client{
		Transport: transport,
	}, credential)
	return registry.NewRegistry(endpoint, &http.Client{
		Transport: registry.NewTransport(transport, authorizer),
	})
}
func newRepository(name, endpoint string, insecure bool, username, password string) (*registry.Repository, error) {
	transport := registry.GetHTTPTransport(insecure)
	credential := auth.NewBasicAuthCredential(username, password)
	authorizer := auth.NewStandardTokenAuthorizer(&http.Client{
		Transport: transport,
	}, credential)
	return registry.NewRepository(name, endpoint, &http.Client{
		Transport: registry.NewTransport(transport, authorizer),
	})
}

//Registry ...
type Registry struct {
	Endpoint string
	Username string
	Password string
	Insecure bool
	client   *http.Client
}

//Init ...
func (r *Registry) Init() {
	transport := registry.GetHTTPTransport(r.Insecure)
	credential := auth.NewBasicAuthCredential(r.Username, r.Password)
	authorizer := auth.NewStandardTokenAuthorizer(&http.Client{
		Transport: transport,
	}, credential)
	r.client = &http.Client{
		Transport: registry.NewTransport(transport, authorizer),
	}
}

//Repository ...
func (r *Registry) Repository(name string) (*registry.Repository, error) {
	return registry.NewRepository(name, r.Endpoint, r.client)
}

//Registry ...
func (r *Registry) Registry() (*registry.Registry, error) {
	return registry.NewRegistry(r.Endpoint, r.client)
}
