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
      updatedAt
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
