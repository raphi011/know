import { gql } from 'graphql-request'

export const LIST_DOCUMENTS = gql`
  query ListDocuments {
    entities(type: "document", limit: 500) {
      id
      name
      updatedAt
    }
  }
`

export const GET_ENTITY = gql`
  query GetEntity($id: ID!) {
    entity(id: $id) {
      id
      name
      content
      labels
      updatedAt
    }
  }
`

export const LIST_CONVERSATIONS = gql`
  query ListConversations($limit: Int) {
    conversations(limit: $limit) {
      id
      title
      entityId
      updatedAt
    }
  }
`

export const GET_CONVERSATION = gql`
  query GetConversation($id: ID!) {
    conversation(id: $id) {
      id
      title
      entityId
      messages {
        id
        role
        content
        createdAt
      }
    }
  }
`

export const CREATE_CONVERSATION = gql`
  mutation CreateConversation($title: String, $entityId: String) {
    createConversation(title: $title, entityId: $entityId) {
      id
      title
      entityId
      updatedAt
    }
  }
`

export const DELETE_CONVERSATION = gql`
  mutation DeleteConversation($id: ID!) {
    deleteConversation(id: $id)
  }
`

// Raw query string (not gql-tagged) for graphql-ws subscription client
export const CHAT_STREAM = `
  subscription ChatStream($conversationId: ID!, $message: String!, $history: [ChatMessageInput!]!, $input: SearchInput) {
    chatStream(conversationId: $conversationId, message: $message, history: $history, input: $input) {
      token
      done
      error
    }
  }
`

export const UPDATE_CONTENT = gql`
  mutation UpdateEntityContent($id: ID!, $content: String!) {
    updateEntityContent(id: $id, content: $content) {
      id
      name
      content
      updatedAt
    }
  }
`
