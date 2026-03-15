import Foundation

enum APIError: LocalizedError {
    case invalidURL
    case unauthorized
    case notFound
    case networkError(Error)
    case decodingError(Error)
    case serverError(Int, String?)

    var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "Invalid server URL"
        case .unauthorized:
            return "Invalid or expired token"
        case .notFound:
            return "Not found"
        case .networkError(let error):
            return "Network error: \(error.localizedDescription)"
        case .decodingError(let error):
            return "Failed to decode response: \(error.localizedDescription)"
        case .serverError(let code, let body):
            if let body, !body.isEmpty {
                return "Server error (HTTP \(code)): \(String(body.prefix(200)))"
            }
            return "Server error (HTTP \(code))"
        }
    }
}
