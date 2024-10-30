package scaffold

import (
	"fmt"
	"strings"
)

type InfraSpec struct {
	Parameters []Parameter
	Services   []ServiceSpec

	// Databases to create
	DbPostgres    *DatabasePostgres
	DbMySql       *DatabaseMySql
	DbCosmosMongo *DatabaseCosmosMongo
	DbRedis       *DatabaseRedis

	AzureServiceBus     *AzureDepServiceBus
	AzureEventHubs      *AzureDepEventHubs
	AzureStorageAccount *AzureDepStorageAccount
}

type Parameter struct {
	Name   string
	Value  any
	Type   string
	Secret bool
}

type DatabasePostgres struct {
	DatabaseUser              string
	DatabaseName              string
	AuthUsingManagedIdentity  bool
	AuthUsingUsernamePassword bool
}

type DatabaseMySql struct {
	DatabaseUser              string
	DatabaseName              string
	AuthUsingManagedIdentity  bool
	AuthUsingUsernamePassword bool
}

type DatabaseCosmosMongo struct {
	DatabaseName string
}

type DatabaseRedis struct {
}

type AzureDepServiceBus struct {
	Name                      string
	Queues                    []string
	TopicsAndSubscriptions    map[string][]string
	AuthUsingConnectionString bool
	AuthUsingManagedIdentity  bool
}

type AzureDepEventHubs struct {
	Name                      string
	EventHubNames             []string
	AuthUsingConnectionString bool
	AuthUsingManagedIdentity  bool
}

type AzureDepStorageAccount struct {
	Name                      string
	ContainerNames            []string
	AuthUsingConnectionString bool
	AuthUsingManagedIdentity  bool
}

// AuthType defines different authentication types.
type AuthType int32

const (
	AUTH_TYPE_UNSPECIFIED AuthType = 0
	// Username and password, or key based authentication, or connection string
	AuthType_PASSWORD AuthType = 1
	// Microsoft EntraID token credential
	AuthType_TOKEN_CREDENTIAL AuthType = 2
)

type ServiceSpec struct {
	Name string
	Port int

	// Front-end properties.
	Frontend *Frontend

	// Back-end properties
	Backend *Backend

	// Connection to a database
	DbPostgres    *DatabaseReference
	DbMySql       *DatabaseReference
	DbCosmosMongo *DatabaseReference
	DbRedis       *DatabaseReference

	AzureServiceBus     *AzureDepServiceBus
	AzureEventHubs      *AzureDepEventHubs
	AzureStorageAccount *AzureDepStorageAccount
}

type Frontend struct {
	Backends []ServiceReference
}

type Backend struct {
	Frontends []ServiceReference
}

type ServiceReference struct {
	Name string
}

type DatabaseReference struct {
	DatabaseName              string
	AuthUsingManagedIdentity  bool
	AuthUsingUsernamePassword bool
}

func containerAppExistsParameter(serviceName string) Parameter {
	return Parameter{
		Name: BicepName(serviceName) + "Exists",
		Value: fmt.Sprintf("${SERVICE_%s_RESOURCE_EXISTS=false}",
			strings.ReplaceAll(strings.ToUpper(serviceName), "-", "_")),
		Type: "bool",
	}
}

type serviceDef struct {
	Settings []serviceDefSettings `json:"settings"`
}

type serviceDefSettings struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	Secret       bool   `json:"secret,omitempty"`
	SecretRef    string `json:"secretRef,omitempty"`
	CommentName  string `json:"_comment_name,omitempty"`
	CommentValue string `json:"_comment_value,omitempty"`
}

func serviceDefPlaceholder(serviceName string) Parameter {
	return Parameter{
		Name: BicepName(serviceName) + "Definition",
		Value: serviceDef{
			Settings: []serviceDefSettings{
				{
					Name:        "",
					Value:       "${VAR}",
					CommentName: "The name of the environment variable when running in Azure. If empty, ignored.",
					//nolint:lll
					CommentValue: "The value to provide. This can be a fixed literal, or an expression like ${VAR} to use the value of 'VAR' from the current environment.",
				},
				{
					Name:        "",
					Value:       "${VAR_S}",
					Secret:      true,
					CommentName: "The name of the environment variable when running in Azure. If empty, ignored.",
					//nolint:lll
					CommentValue: "The value to provide. This can be a fixed literal, or an expression like ${VAR_S} to use the value of 'VAR_S' from the current environment.",
				},
			},
		},
		Type:   "object",
		Secret: true,
	}
}
