import Foundation

/// Lightweight GraphQL client. Appends /query to the base URL and uses Bearer token auth.
actor GraphQLClient {
    let baseURL: URL
    let token: String

    init(baseURL: URL, token: String) {
        self.baseURL = baseURL
        self.token = token
    }

    func execute<T: Codable>(
        query: String,
        variables: [String: Any]? = nil
    ) async throws -> T {
        let url = baseURL.appendingPathComponent("query")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")

        var body: [String: Any] = ["query": query]
        if let variables {
            body["variables"] = variables
        }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await URLSession.shared.data(for: request)
        } catch {
            throw APIError.networkError(error)
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            throw APIError.serverError(0, nil)
        }

        switch httpResponse.statusCode {
        case 200: break
        case 401: throw APIError.unauthorized
        default:
            let bodyString = String(data: data, encoding: .utf8)
            throw APIError.serverError(httpResponse.statusCode, bodyString)
        }

        let decoded: GraphQLResponse<T>
        do {
            decoded = try JSONDecoder().decode(GraphQLResponse<T>.self, from: data)
        } catch {
            throw APIError.decodingError(error)
        }

        // A 200 response can still contain GraphQL-level errors
        if let errors = decoded.errors, !errors.isEmpty {
            if errors.contains(where: { $0.message.lowercased().contains("unauthorized") || $0.message.lowercased().contains("invalid token") }) {
                throw APIError.unauthorized
            }
            throw APIError.graphQLErrors(errors)
        }

        guard let result = decoded.data else {
            throw APIError.noData
        }

        return result
    }
}
