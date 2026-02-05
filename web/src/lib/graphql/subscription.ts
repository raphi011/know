import { createClient } from 'graphql-ws'

const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'

export const wsClient = createClient({
  url: `${proto}//${location.host}/query`,
})
