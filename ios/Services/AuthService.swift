import Foundation

enum AuthState {
    case unauthenticated
    case authenticated(user: Me, client: GraphQLClient)
}

/// Manages authentication credentials and login validation.
@MainActor
@Observable
final class AuthService {
    private(set) var state: AuthState = .unauthenticated

    var isAuthenticated: Bool {
        if case .authenticated = state { return true }
        return false
    }

    var currentUser: Me? {
        if case .authenticated(let user, _) = state { return user }
        return nil
    }

    var client: GraphQLClient? {
        if case .authenticated(_, let client) = state { return client }
        return nil
    }

    private static let serverURLKey = "serverURL"
    private static let tokenKey = "token"

    // Both URL and token stored in Keychain for simplified session restore
    var serverURL: String {
        (try? Keychain.load(key: Self.serverURLKey)) ?? ""
    }

    func login(serverURL: String, token: String) async throws {
        guard let url = URL(string: serverURL) else {
            throw APIError.invalidURL
        }

        let newClient = GraphQLClient(baseURL: url, token: token)
        let response: MeResponse = try await newClient.execute(query: Queries.me)

        try Keychain.save(key: Self.serverURLKey, value: serverURL)
        try Keychain.save(key: Self.tokenKey, value: token)

        state = .authenticated(user: response.me, client: newClient)
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

        let storedClient = GraphQLClient(baseURL: url, token: token)

        do {
            let response: MeResponse = try await storedClient.execute(query: Queries.me)
            state = .authenticated(user: response.me, client: storedClient)
            return nil
        } catch {
            if case APIError.unauthorized = error {
                // Token expired -- clear stale credentials
                try? Keychain.delete(key: Self.serverURLKey)
                try? Keychain.delete(key: Self.tokenKey)
                return nil
            }
            return error
        }
    }

    func logout() {
        // Best-effort keychain cleanup -- in-memory state is always cleared
        try? Keychain.delete(key: Self.serverURLKey)
        try? Keychain.delete(key: Self.tokenKey)
        state = .unauthenticated
    }
}
