import Foundation

struct Document: Codable, Identifiable {
    let id: String
    let vaultId: String
    let path: String
    let title: String
    let content: String
    let contentBody: String?
    let labels: [String]
    let docType: String?
    let contentHash: String?
    let createdAt: Date
    let updatedAt: Date

    /// Returns contentBody if available, otherwise strips YAML frontmatter from content.
    var displayBody: String {
        if let contentBody, !contentBody.isEmpty {
            return contentBody
        }
        // Strip frontmatter (--- ... ---) from raw content
        if content.hasPrefix("---") {
            let lines = content.components(separatedBy: "\n")
            if let endIndex = lines.dropFirst().firstIndex(where: { $0 == "---" }) {
                return lines[(endIndex + 1)...].joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
            }
        }
        return content
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        vaultId = try container.decode(String.self, forKey: .vaultId)
        path = try container.decode(String.self, forKey: .path)
        title = try container.decode(String.self, forKey: .title)
        content = try container.decode(String.self, forKey: .content)
        contentBody = try container.decodeIfPresent(String.self, forKey: .contentBody)
        labels = try container.decodeIfPresent([String].self, forKey: .labels) ?? []
        docType = try container.decodeIfPresent(String.self, forKey: .docType)
        contentHash = try container.decodeIfPresent(String.self, forKey: .contentHash)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
    }
}

struct SearchResult: Codable, Identifiable {
    let documentId: String?
    let path: String
    let title: String
    let labels: [String]
    let docType: String?
    let score: Double
    let matchedChunks: [ChunkMatch]

    var id: String { path }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        documentId = try container.decodeIfPresent(String.self, forKey: .documentId)
        path = try container.decode(String.self, forKey: .path)
        title = try container.decode(String.self, forKey: .title)
        labels = try container.decodeIfPresent([String].self, forKey: .labels) ?? []
        docType = try container.decodeIfPresent(String.self, forKey: .docType)
        score = try container.decode(Double.self, forKey: .score)
        matchedChunks = try container.decode([ChunkMatch].self, forKey: .matchedChunks)
    }
}

struct ChunkMatch: Codable, Identifiable {
    let snippet: String
    let headingPath: String?
    let position: Int
    let score: Double

    var id: Int { position }
}

// MARK: - Incremental Sync

struct ChangesResponse: Codable {
    let updated: [FileChange]
    let deleted: [FileChange]
    let syncToken: String
    let truncated: Bool

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        updated = try container.decodeIfPresent([FileChange].self, forKey: .updated) ?? []
        deleted = try container.decodeIfPresent([FileChange].self, forKey: .deleted) ?? []
        syncToken = try container.decode(String.self, forKey: .syncToken)
        truncated = try container.decodeIfPresent(Bool.self, forKey: .truncated) ?? false
    }
}

struct FileChange: Codable, Identifiable {
    let fileId: String
    let path: String
    let contentHash: String?
    let updatedAt: Date

    var id: String { fileId }
}
