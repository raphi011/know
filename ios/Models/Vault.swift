import Foundation

struct Vault: Codable, Identifiable, Hashable {
    let id: String
    let name: String
    let description: String?
    let createdAt: String
    let updatedAt: String
    let labels: [String]
    let documents: [DocumentSummary]?
    let folders: [Folder]?

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }

    static func == (lhs: Vault, rhs: Vault) -> Bool {
        lhs.id == rhs.id
    }
}

struct Folder: Codable, Identifiable, Hashable {
    let id: String
    let path: String
    let name: String
    let createdAt: String
}

/// Lightweight document representation used in vault listings (no content field).
struct DocumentSummary: Codable, Identifiable, Hashable {
    let id: String
    let path: String
    let title: String
    let labels: [String]
    let docType: String?
    let updatedAt: String
}
