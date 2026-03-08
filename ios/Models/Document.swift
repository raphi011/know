import Foundation

struct Document: Codable, Identifiable {
    let id: String
    let vaultId: String
    let path: String
    let title: String
    let content: String
    let contentBody: String
    let labels: [String]
    let docType: String?
    let source: String
    let createdAt: String
    let updatedAt: String
}

struct SearchResult: Codable, Identifiable {
    let documentId: String
    let path: String
    let title: String
    let labels: [String]
    let docType: String?
    let score: Double
    let matchedChunks: [ChunkMatch]

    var id: String { documentId }
}

struct ChunkMatch: Codable, Identifiable {
    let snippet: String
    let headingPath: String?
    let position: Int
    let score: Double

    var id: Int { position }
}
