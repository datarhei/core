/*
Package coreclient provides a client for the datarhei Core API (https://github.com/datarhei/core)

Example for retrieving a list of all processes:

	import "github.com/datarhei/core-client-go/v16"

	client, err := coreclient.New(coreclient.Config{
		Address: "https://example.com:8080",
		Username: "foo",
		Password: "bar",
	})
	if err != nil {
		...
	}

	processes, err := client.ProcessList(coreclient.ProcessListOptions{})
	if err != nil {
		...
	}
*/
package coreclient
