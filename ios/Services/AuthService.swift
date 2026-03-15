import Foundation
import OSLog

private let logger = Logger(subsystem: "com.know", category: "AuthService")

/// Manages authentication credentials and login validation.
@MainActor
@Observable
final class AuthService {
    private(set) var isAuthenticated = false
    private(set) var client: RESTClient?

    private static let serverURLKey = "serverURL"
    private static let tokenKey = "token"

    var serverURL: String {
        do {
            return try Keychain.load(key: Self.serverURLKey) ?? ""
        } catch {
            logger.warning("Failed to load serverURL from Keychain: \(error)")
            return ""
        }
    }

    func login(serverURL: String, token: String) async throws {
        guard let url = URL(string: serverURL) else {
            throw APIError.invalidURL
        }

        let newClient = RESTClient(baseURL: url, token: token)

        // Validate by fetching vaults — if the token is invalid, this throws unauthorized
        try await newClient.validate(path: "api/vaults")

        try Keychain.save(key: Self.serverURLKey, value: serverURL)
        try Keychain.save(key: Self.tokenKey, value: token)

        client = newClient
        isAuthenticated = true
    }

    /// Attempts to restore a previous session from stored credentials.
    /// Returns nil on success or no stored credentials, or an Error for transient failures.
    func restoreSession() async -> Error? {
        let serverURL: String?
        let token: String?
        do {
            serverURL = try Keychain.load(key: Self.serverURLKey)
            token = try Keychain.load(key: Self.tokenKey)
        } catch {
            return error
        }

        guard let serverURL, let token, let url = URL(string: serverURL) else {
            return nil
        }

        let storedClient = RESTClient(baseURL: url, token: token)

        do {
            try await storedClient.validate(path: "api/vaults")
            client = storedClient
            isAuthenticated = true
            return nil
        } catch {
            if case APIError.unauthorized = error {
                logger.info("Stored token expired or revoked, clearing credentials")
                do {
                    try Keychain.delete(key: Self.serverURLKey)
                    try Keychain.delete(key: Self.tokenKey)
                } catch {
                    logger.error("Failed to delete credentials from Keychain: \(error)")
                }
                return nil
            }
            return error
        }
    }

    func logout() {
        do {
            try Keychain.delete(key: Self.serverURLKey)
            try Keychain.delete(key: Self.tokenKey)
        } catch {
            logger.error("Failed to delete credentials during logout: \(error)")
        }
        client = nil
        isAuthenticated = false
    }
}
