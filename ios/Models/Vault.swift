import Foundation

struct Vault: Codable, Identifiable, Hashable, Equatable {
    let id: String
    let name: String
    let description: String?
    let createdBy: String
    let createdAt: Date
    let updatedAt: Date
}

/// File entry from GET /api/ls.
struct FileEntry: Codable, Identifiable {
    var id: String { path }

    let name: String
    let path: String
    let isDir: Bool
    let size: Int?
}
