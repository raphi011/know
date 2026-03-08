import Foundation

struct User: Codable, Identifiable {
    let id: String
    let name: String
    let email: String?
    let createdAt: String
}

struct Me: Codable {
    let user: User
    let vaultAccess: [String]
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
