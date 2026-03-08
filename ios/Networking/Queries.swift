import Foundation

/// GraphQL query string constants matching the Knowhow server schema.
enum Queries {
    static let me = """
    query Me {
        me {
            user {
                id
                name
                email
                createdAt
            }
            vaultRoles {
                vaultId
                role
            }
        }
    }
    """

    static let vaults = """
    query Vaults {
        vaults {
            id
            name
            description
            createdAt
            updatedAt
            labels
        }
    }
    """

    static let vault = """
    query Vault($id: ID!, $folder: String) {
        vault(id: $id) {
            id
            name
            description
            createdAt
            updatedAt
            labels
            folders(parent: $folder) {
                id
                path
                name
                createdAt
            }
            documents(folder: $folder) {
                id
                path
                title
                labels
                docType
                updatedAt
            }
        }
    }
    """

    static let document = """
    query Document($vaultId: ID!, $path: String!) {
        document(vaultId: $vaultId, path: $path) {
            id
            vaultId
            path
            title
            content
            contentBody
            labels
            docType
            source
            createdAt
            updatedAt
        }
    }
    """

    static let documentById = """
    query DocumentById($id: ID!) {
        documentById(id: $id) {
            id
            vaultId
            path
            title
            content
            contentBody
            labels
            docType
            source
            createdAt
            updatedAt
        }
    }
    """

    static let search = """
    query Search($input: SearchInput!) {
        search(input: $input) {
            documentId
            path
            title
            labels
            docType
            score
            matchedChunks {
                snippet
                headingPath
                position
                score
            }
        }
    }
    """
}
