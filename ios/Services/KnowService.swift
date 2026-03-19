import Foundation

/// High-level API methods wrapping RESTClient for typed access to Know data.
final class KnowService: Sendable {
	private let client: RESTClient

	init(client: RESTClient) {
		self.client = client
	}

	var baseURL: URL {
		get async { await client.baseURL }
	}

	var token: String {
		get async { await client.token }
	}

	func fetchVaults() async throws -> [Vault] {
		try await client.get(path: "api/vaults")
	}

	func fetchDocument(vaultId: String, path: String) async throws -> Document {
		try await client.get(
			path: "api/documents",
			query: ["vault": vaultId, "path": path]
		)
	}

	func listFiles(vaultId: String, recursive: Bool = false) async throws -> [FileEntry] {
		var query = ["vault": vaultId]
		if recursive {
			query["recursive"] = "true"
		}
		return try await client.get(path: "api/ls", query: query)
	}

	func createDocument(vaultId: String, path: String, content: String = "") async throws -> Document {
		struct Body: Encodable {
			let vaultId: String
			let path: String
			let content: String
		}
		return try await client.post(
			path: "api/documents",
			body: Body(vaultId: vaultId, path: path, content: content)
		)
	}

	func fetchChanges(vaultId: String, since: Date) async throws -> ChangesResponse {
		let formatter = ISO8601DateFormatter()
		formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
		let sinceStr = formatter.string(from: since)
		return try await client.get(
			path: "api/vaults/\(vaultId)/changes",
			query: ["since": sinceStr]
		)
	}

	func search(vaultId: String, query: String, labels: [String]? = nil, limit: Int? = nil) async throws -> [SearchResult] {
		var params = ["vault": vaultId, "query": query]
		if let labels, !labels.isEmpty {
			params["labels"] = labels.joined(separator: ",")
		}
		if let limit {
			params["limit"] = String(limit)
		}
		return try await client.get(path: "api/search", query: params)
	}
}
