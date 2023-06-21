// Code generated by swaggo/swag. DO NOT EDIT.

package docs

import "github.com/swaggo/swag"

const docTemplateClusterAPI = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {
            "name": "datarhei Core Support",
            "url": "https://www.datarhei.com",
            "email": "hello@datarhei.com"
        },
        "license": {
            "name": "Apache 2.0",
            "url": "https://github.com/datarhei/core/v16/blob/main/LICENSE"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/v1/core": {
            "get": {
                "description": "Core API address and login of this node",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Core API address and login",
                "operationId": "cluster-1-core-api-address",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/cluster.Error"
                            }
                        }
                    }
                }
            }
        },
        "/v1/iam/user": {
            "post": {
                "description": "Add an identity to the cluster DB",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Add an identity",
                "operationId": "cluster-1-add-identity",
                "parameters": [
                    {
                        "description": "Identity config",
                        "name": "config",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.AddIdentityRequest"
                        }
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/iam/user/{name}": {
            "put": {
                "description": "Replace an existing identity in the cluster DB",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Replace an existing identity",
                "operationId": "cluster-1-update-identity",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Process ID",
                        "name": "name",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Identity config",
                        "name": "config",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.UpdateIdentityRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            },
            "delete": {
                "description": "Remove an identity from the cluster DB",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Remove an identity",
                "operationId": "cluster-1-remove-identity",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identity name",
                        "name": "name",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/iam/user/{name}/policies": {
            "put": {
                "description": "Set policies for an identity in the cluster DB. Any existing policies will be replaced.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Set identity policies",
                "operationId": "cluster-3-set-identity-policies",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Process ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Policies for that user",
                        "name": "data",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.SetPoliciesRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/process": {
            "post": {
                "description": "Add a process to the cluster DB",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Add a process",
                "operationId": "cluster-1-add-process",
                "parameters": [
                    {
                        "description": "Process config",
                        "name": "config",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.AddProcessRequest"
                        }
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/process/{id}": {
            "put": {
                "description": "Replace an existing process in the cluster DB",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Replace an existing process",
                "operationId": "cluster-1-update-process",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Process ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Domain to act on",
                        "name": "domain",
                        "in": "query"
                    },
                    {
                        "description": "Process config",
                        "name": "config",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.UpdateProcessRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            },
            "delete": {
                "description": "Remove a process from the cluster DB",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Remove a process",
                "operationId": "cluster-1-remove-process",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Process ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Domain to act on",
                        "name": "domain",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/process/{id}/metadata/{key}": {
            "put": {
                "description": "Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Add JSON metadata with a process under the given key",
                "operationId": "cluster-3-set-process-metadata",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Process ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Key for data store",
                        "name": "key",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Domain to act on",
                        "name": "domain",
                        "in": "query"
                    },
                    {
                        "description": "Arbitrary JSON data. The null value will remove the key and its contents",
                        "name": "data",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.SetProcessMetadataRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/server": {
            "post": {
                "description": "Add a new server to the cluster",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Add a new server",
                "operationId": "cluster-1-add-server",
                "parameters": [
                    {
                        "description": "Server ID and address",
                        "name": "config",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/client.JoinRequest"
                        }
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/server/{id}": {
            "delete": {
                "description": "Remove a server from the cluster",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Remove a server",
                "operationId": "cluster-1-remove-server",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Server ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Origin ID of request",
                        "name": "X-Cluster-Origin",
                        "in": "header"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    },
                    "508": {
                        "description": "Loop Detected",
                        "schema": {
                            "$ref": "#/definitions/cluster.Error"
                        }
                    }
                }
            }
        },
        "/v1/snapshot": {
            "get": {
                "description": "Current snapshot of the clusterDB",
                "produces": [
                    "application/octet-stream"
                ],
                "tags": [
                    "v1.0.0"
                ],
                "summary": "Cluster DB snapshot",
                "operationId": "cluster-1-snapshot",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "file"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/cluster.Error"
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "access.Policy": {
            "type": "object",
            "properties": {
                "actions": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "domain": {
                    "type": "string"
                },
                "name": {
                    "type": "string"
                },
                "resource": {
                    "type": "string"
                }
            }
        },
        "app.Config": {
            "type": "object",
            "properties": {
                "autostart": {
                    "type": "boolean"
                },
                "domain": {
                    "type": "string"
                },
                "ffversion": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "input": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/app.ConfigIO"
                    }
                },
                "limitCPU": {
                    "description": "percent",
                    "type": "number"
                },
                "limitMemory": {
                    "description": "bytes",
                    "type": "integer"
                },
                "limitWaitFor": {
                    "description": "seconds",
                    "type": "integer"
                },
                "logPatterns": {
                    "description": "will we interpreted as regular expressions",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "options": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "output": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/app.ConfigIO"
                    }
                },
                "owner": {
                    "type": "string"
                },
                "reconnect": {
                    "type": "boolean"
                },
                "reconnectDelay": {
                    "description": "seconds",
                    "type": "integer"
                },
                "reference": {
                    "type": "string"
                },
                "scheduler": {
                    "description": "crontab pattern or RFC3339 timestamp",
                    "type": "string"
                },
                "staleTimeout": {
                    "description": "seconds",
                    "type": "integer"
                },
                "timeout": {
                    "description": "seconds",
                    "type": "integer"
                }
            }
        },
        "app.ConfigIO": {
            "type": "object",
            "properties": {
                "address": {
                    "type": "string"
                },
                "cleanup": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/app.ConfigIOCleanup"
                    }
                },
                "id": {
                    "type": "string"
                },
                "options": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "app.ConfigIOCleanup": {
            "type": "object",
            "properties": {
                "maxFileAge": {
                    "type": "integer"
                },
                "maxFiles": {
                    "type": "integer"
                },
                "pattern": {
                    "type": "string"
                },
                "purgeOnDelete": {
                    "type": "boolean"
                }
            }
        },
        "client.AddIdentityRequest": {
            "type": "object",
            "properties": {
                "identity": {
                    "$ref": "#/definitions/identity.User"
                }
            }
        },
        "client.AddProcessRequest": {
            "type": "object",
            "properties": {
                "config": {
                    "$ref": "#/definitions/app.Config"
                }
            }
        },
        "client.JoinRequest": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                },
                "raft_address": {
                    "type": "string"
                }
            }
        },
        "client.SetPoliciesRequest": {
            "type": "object",
            "properties": {
                "policies": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/access.Policy"
                    }
                }
            }
        },
        "client.SetProcessMetadataRequest": {
            "type": "object",
            "properties": {
                "metadata": {}
            }
        },
        "client.UpdateIdentityRequest": {
            "type": "object",
            "properties": {
                "identity": {
                    "$ref": "#/definitions/identity.User"
                }
            }
        },
        "client.UpdateProcessRequest": {
            "type": "object",
            "properties": {
                "config": {
                    "$ref": "#/definitions/app.Config"
                }
            }
        },
        "cluster.Error": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "integer",
                    "format": "int"
                },
                "details": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "message": {
                    "type": "string"
                }
            }
        },
        "identity.Auth0Tenant": {
            "type": "object",
            "properties": {
                "audience": {
                    "type": "string"
                },
                "client_id": {
                    "type": "string"
                },
                "domain": {
                    "type": "string"
                }
            }
        },
        "identity.User": {
            "type": "object",
            "properties": {
                "auth": {
                    "$ref": "#/definitions/identity.UserAuth"
                },
                "name": {
                    "type": "string"
                },
                "superuser": {
                    "type": "boolean"
                }
            }
        },
        "identity.UserAuth": {
            "type": "object",
            "properties": {
                "api": {
                    "$ref": "#/definitions/identity.UserAuthAPI"
                },
                "services": {
                    "$ref": "#/definitions/identity.UserAuthServices"
                }
            }
        },
        "identity.UserAuthAPI": {
            "type": "object",
            "properties": {
                "auth0": {
                    "$ref": "#/definitions/identity.UserAuthAPIAuth0"
                },
                "password": {
                    "type": "string"
                }
            }
        },
        "identity.UserAuthAPIAuth0": {
            "type": "object",
            "properties": {
                "tenant": {
                    "$ref": "#/definitions/identity.Auth0Tenant"
                },
                "user": {
                    "type": "string"
                }
            }
        },
        "identity.UserAuthServices": {
            "type": "object",
            "properties": {
                "basic": {
                    "description": "Passwords for BasicAuth",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "session": {
                    "description": "Secrets for session JWT",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "token": {
                    "description": "Tokens/Streamkey for RTMP and SRT",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        }
    }
}`

// SwaggerInfoClusterAPI holds exported Swagger Info so clients can modify it
var SwaggerInfoClusterAPI = &swag.Spec{
	Version:          "1.0",
	Host:             "",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "datarhei Core Cluster API",
	Description:      "Internal REST API for the datarhei Core cluster",
	InfoInstanceName: "ClusterAPI",
	SwaggerTemplate:  docTemplateClusterAPI,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfoClusterAPI.InstanceName(), SwaggerInfoClusterAPI)
}
