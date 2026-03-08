import Foundation

enum APIError: LocalizedError {
    case invalidURL
    case unauthorized
    case networkError(Error)
    case graphQLErrors([GraphQLError])
    case decodingError(Error)
    case noData
    case serverError(Int, String?)

    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid server URL"
        case .unauthorized:
            return "Invalid or expired token"
        case .networkError(let error):
            return "Network error: \(error.localizedDescription)"
        case .graphQLErrors(let errors):
            return errors.map(\.message).joined(separator: "\n")
        case .decodingError(let error):
            return "Failed to decode response: \(error.localizedDescription)"
        case .noData:
            return "No data returned"
        case .serverError(let code, let body):
            if let body, !body.isEmpty {
                return "Server error (HTTP \(code)): \(String(body.prefix(200)))"
            }
            return "Server error (HTTP \(code))"
        }
    }
}
