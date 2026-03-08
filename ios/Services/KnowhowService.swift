import Foundation

/// High-level API methods wrapping GraphQLClient for typed access to Knowhow data.
final class KnowhowService: Sendable {
    private let client: GraphQLClient

    init(client: GraphQLClient) {
        self.client = client
    }

    func fetchVaults() async throws -> [Vault] {
        let response: VaultsResponse = try await client.execute(query: Queries.vaults)
        return response.vaults
    }

    func fetchVault(id: String, folder: String? = nil) async throws -> Vault {
        var variables: [String: Any] = ["id": id]
        if let folder {
            variables["folder"] = folder
        }

        let response: VaultResponse = try await client.execute(
            query: Queries.vault,
            variables: variables
        )

        guard let vault = response.vault else {
            throw APIError.noData
        }
        return vault
    }

    func fetchDocument(vaultId: String, path: String) async throws -> Document {
        let response: DocumentResponse = try await client.execute(
            query: Queries.document,
            variables: ["vaultId": vaultId, "path": path]
        )

        guard let document = response.document else {
            throw APIError.noData
        }
        return document
    }

    func fetchDocumentById(id: String) async throws -> Document {
        let response: DocumentByIdResponse = try await client.execute(
            query: Queries.documentById,
            variables: ["id": id]
        )

        guard let document = response.documentById else {
            throw APIError.noData
        }
        return document
    }

    func search(vaultId: String, query: String, labels: [String]? = nil, folder: String? = nil, limit: Int? = nil) async throws -> [SearchResult] {
        var input: [String: Any] = [
            "vaultId": vaultId,
            "query": query,
        ]
        if let labels { input["labels"] = labels }
        if let folder { input["folder"] = folder }
        if let limit { input["limit"] = limit }

        let response: SearchResponse = try await client.execute(
            query: Queries.search,
            variables: ["input": input]
        )

        return response.search
    }
}
