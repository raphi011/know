package main

import "github.com/raphi011/knowhow/internal/graphqlclient"

func newGQLClient(url, token string) *graphqlclient.Client {
	return graphqlclient.New(url, token)
}
