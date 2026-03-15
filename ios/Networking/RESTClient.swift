import Foundation

/// Lightweight REST client with Bearer token auth.
actor RESTClient {
	let baseURL: URL
	let token: String

	private let decoder: JSONDecoder = {
		let d = JSONDecoder()
		d.dateDecodingStrategy = .custom { decoder in
			let container = try decoder.singleValueContainer()
			let string = try container.decode(String.self)

			let formatter = ISO8601DateFormatter()
			formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
			if let date = formatter.date(from: string) { return date }

			formatter.formatOptions = [.withInternetDateTime]
			if let date = formatter.date(from: string) { return date }

			throw DecodingError.dataCorruptedError(
				in: container, debugDescription: "Cannot parse date: \(string)"
			)
		}
		return d
	}()

	init(baseURL: URL, token: String) {
		self.baseURL = baseURL
		self.token = token
	}

	func get<T: Decodable>(
		path: String,
		query: [String: String] = [:]
	) async throws -> T {
		let url = try buildURL(path: path, query: query)
		var request = URLRequest(url: url)
		request.httpMethod = "GET"
		applyHeaders(&request)
		return try await execute(request)
	}

	func post<T: Decodable>(
		path: String,
		body: some Encodable
	) async throws -> T {
		let url = try buildURL(path: path)
		var request = URLRequest(url: url)
		request.httpMethod = "POST"
		applyHeaders(&request)
		request.setValue("application/json", forHTTPHeaderField: "Content-Type")
		request.httpBody = try JSONEncoder().encode(body)
		return try await execute(request)
	}

	/// Validates the token by performing a GET request. Throws on auth failure; ignores the response body.
	func validate(path: String) async throws {
		let url = try buildURL(path: path)
		var request = URLRequest(url: url)
		request.httpMethod = "GET"
		applyHeaders(&request)

		let data: Data
		let response: URLResponse
		do {
			(data, response) = try await URLSession.shared.data(for: request)
		} catch {
			throw APIError.networkError(error)
		}

		try checkResponse(data, response)
	}

	// MARK: - Private

	private func applyHeaders(_ request: inout URLRequest) {
		request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
	}

	private func buildURL(path: String, query: [String: String] = [:]) throws -> URL {
		guard var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false) else {
			throw APIError.invalidURL
		}
		if !query.isEmpty {
			components.queryItems = query.map { URLQueryItem(name: $0.key, value: $0.value) }
		}
		guard let url = components.url else {
			throw APIError.invalidURL
		}
		return url
	}

	/// Checks HTTP response status code and throws APIError for non-success codes.
	private func checkResponse(_ data: Data, _ response: URLResponse) throws {
		guard let http = response as? HTTPURLResponse else {
			throw APIError.serverError(0, nil)
		}

		switch http.statusCode {
		case 200..<300: return
		case 401: throw APIError.unauthorized
		case 404: throw APIError.notFound
		default:
			let body = String(data: data, encoding: .utf8)
			throw APIError.serverError(http.statusCode, body)
		}
	}

	private func execute<T: Decodable>(_ request: URLRequest) async throws -> T {
		let data: Data
		let response: URLResponse
		do {
			(data, response) = try await URLSession.shared.data(for: request)
		} catch is CancellationError {
			throw CancellationError()
		} catch let error as URLError where error.code == .cancelled {
			throw CancellationError()
		} catch {
			throw APIError.networkError(error)
		}

		try checkResponse(data, response)

		do {
			return try decoder.decode(T.self, from: data)
		} catch {
			throw APIError.decodingError(error)
		}
	}
}
