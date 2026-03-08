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
