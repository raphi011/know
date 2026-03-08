import Foundation

// MARK: - GraphQL Response Wrappers

struct MeResponse: Codable {
    let me: Me
}

struct VaultsResponse: Codable {
    let vaults: [Vault]
}

struct VaultResponse: Codable {
    let vault: Vault?
}

struct DocumentResponse: Codable {
    let document: Document?
}

struct DocumentByIdResponse: Codable {
    let documentById: Document?
}

struct SearchResponse: Codable {
    let search: [SearchResult]
}

struct SyncMetadataResponse: Codable {
    let syncMetadata: SyncMetadataResult
}

struct SyncMetadataResult: Codable {
    let documents: [SyncMetaItem]
    let tombstones: [SyncTombstoneItem]
    let hasMore: Bool
}

struct SyncMetaItem: Codable {
    let id: String
    let path: String
    let contentHash: String?
    let updatedAt: String
}

struct SyncTombstoneItem: Codable {
    let docId: String
    let path: String
    let deletedAt: String
}
