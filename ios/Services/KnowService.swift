import Foundation

/// High-level API methods wrapping GraphQLClient for typed access to Know data.
final class KnowService: Sendable {
    private let client: GraphQLClient
    let baseURL: URL
    let token: String

    init(client: GraphQLClient) {
        self.client = client
        self.baseURL = client.baseURL
        self.token = client.token
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

    func fetchSyncMetadata(vaultId: String, since: Date? = nil, limit: Int? = nil, offset: Int? = nil) async throws -> SyncMetadataResult {
        var variables: [String: Any] = ["vaultId": vaultId]
        if let since {
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            variables["since"] = formatter.string(from: since)
        }
        if let limit { variables["limit"] = limit }
        if let offset { variables["offset"] = offset }

        let response: SyncMetadataResponse = try await client.execute(
            query: Queries.syncMetadata,
            variables: variables
        )
        return response.syncMetadata
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
