package main

import (
	"strings"

	"github.com/raphi011/knowhow/internal/graphqlclient"
)

func newGQLClient(inst Instance) *graphqlclient.Client {
	url := strings.TrimSuffix(inst.URL, "/")
	if !strings.HasSuffix(url, "/query") {
		url += "/query"
	}
	return graphqlclient.New(url, inst.Token)
}
