import { GraphQLClient } from 'graphql-request'

// graphql-request v7 requires absolute URLs
const endpoint = typeof window !== 'undefined'
  ? `${window.location.origin}/query`
  : '/query'

export const client = new GraphQLClient(endpoint)
