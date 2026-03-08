import Foundation

struct User: Codable, Identifiable {
    let id: String
    let name: String
    let email: String?
    let createdAt: String
}

enum Role: String, Codable {
    case read
    case write
    case admin
}

struct VaultRole: Codable {
    let vaultId: String
    let role: Role
}

struct Me: Codable {
    let user: User
    let vaultRoles: [VaultRole]
}

// MARK: - GraphQL Types

struct GraphQLResponse<T: Codable>: Codable {
    let data: T?
    let errors: [GraphQLError]?
}

struct GraphQLError: Codable, LocalizedError {
    let message: String

    var errorDescription: String? { message }
}
