import Foundation
import SwiftData
import OSLog

private let logger = Logger(subsystem: "com.know", category: "SyncEngine")

/// Orchestrates metadata sync, on-demand content fetching, and SSE streaming.
@MainActor
@Observable
final class SyncEngine {
	enum Status {
		case idle
		case syncing(String)
		case error(String)
	}

	enum SyncError: LocalizedError {
		case notConfigured

		var errorDescription: String? {
			switch self {
			case .notConfigured: return "Sync engine not configured"
			}
		}
	}

	private(set) var status: Status = .idle
	private(set) var service: KnowService?
	private var sseTask: Task<Void, Never>?

	func configure(service: KnowService?) {
		self.service = service
	}

	// MARK: - Metadata Sync

	/// Fetches all file paths via GET /api/ls and reconciles with local SwiftData cache.
	func performMetadataSync(vaultId: String, modelContext: ModelContext) async {
		guard let service else {
			logger.warning("SyncEngine: no service configured")
			return
		}

		status = .syncing("Syncing metadata...")
		defer { if case .syncing = status { status = .idle } }

		do {
			let entries = try await service.listFiles(vaultId: vaultId, recursive: true)
			let filePaths = Set(entries.filter { !$0.isDir }.map(\.path))

			// Fetch all cached documents for this vault
			let vid = vaultId
			let descriptor = FetchDescriptor<CachedDocument>(
				predicate: #Predicate { $0.vaultId == vid }
			)
			let cached = try modelContext.fetch(descriptor)
			let cachedPaths = Set(cached.map(\.path))

			// Insert new documents
			for entry in entries where !entry.isDir && !cachedPaths.contains(entry.path) {
				let compositeId = CachedDocument.compositeId(vaultId: vaultId, path: entry.path)
				let doc = CachedDocument(
					id: compositeId,
					vaultId: vaultId,
					path: entry.path,
					title: CachedDocument.titleFromPath(entry.path)
				)
				modelContext.insert(doc)
			}

			// Delete documents no longer on server
			for doc in cached where !filePaths.contains(doc.path) {
				modelContext.delete(doc)
			}

			// Mark sync complete
			let syncState = try fetchOrCreateSyncState(vaultId: vaultId, modelContext: modelContext)
			syncState.isInitialSyncComplete = true
			syncState.lastSyncedAt = .now
			try modelContext.save()

			logger.info("Metadata sync complete for vault \(vaultId)")
		} catch {
			logger.error("Metadata sync failed: \(error)")
			status = .error(error.localizedDescription)
		}
	}

	// MARK: - Incremental Sync

	/// Fetches only changes since the last sync via GET /changes and applies them.
	/// Returns true on success, false if the caller should fall back to full metadata sync.
	func performIncrementalSync(vaultId: String, modelContext: ModelContext) async -> Bool {
		guard let service else {
			logger.warning("SyncEngine: no service configured")
			return false
		}

		status = .syncing("Catching up...")
		defer { if case .syncing = status { status = .idle } }

		do {
			let syncState = try fetchOrCreateSyncState(vaultId: vaultId, modelContext: modelContext)
			guard let lastSynced = syncState.lastSyncedAt else {
				logger.info("Incremental sync: no lastSyncedAt, falling back to full sync")
				return false
			}

			let changes = try await service.fetchChanges(vaultId: vaultId, since: lastSynced)

			// If results were truncated, fall back to full sync for completeness.
			if changes.truncated {
				logger.info("Incremental sync: results truncated, falling back to full sync")
				return false
			}

			// Fetch all cached documents for this vault to avoid N+1 fetches.
			let vid = vaultId
			let allDescriptor = FetchDescriptor<CachedDocument>(
				predicate: #Predicate { $0.vaultId == vid }
			)
			let cached = try modelContext.fetch(allDescriptor)
			let cachedByCompositeId = Dictionary(uniqueKeysWithValues: cached.map { ($0.id, $0) })

			// Apply updated files
			for change in changes.updated {
				let compositeId = CachedDocument.compositeId(vaultId: vaultId, path: change.path)
				if let existing = cachedByCompositeId[compositeId] {
					existing.contentHash = change.contentHash
					existing.lastSyncedAt = .now
					existing.invalidateContent()
				} else {
					let doc = CachedDocument(
						id: compositeId,
						vaultId: vaultId,
						path: change.path,
						title: CachedDocument.titleFromPath(change.path),
						contentHash: change.contentHash,
						serverUpdatedAt: change.updatedAt,
						lastSyncedAt: .now
					)
					modelContext.insert(doc)
				}
			}

			// Apply deleted files
			for change in changes.deleted {
				let compositeId = CachedDocument.compositeId(vaultId: vaultId, path: change.path)
				if let existing = cachedByCompositeId[compositeId] {
					modelContext.delete(existing)
				}
			}

			// Parse syncToken and update state
			let formatter = ISO8601DateFormatter()
			formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
			guard let tokenDate = formatter.date(from: changes.syncToken) else {
				logger.error("Incremental sync: failed to parse syncToken '\(changes.syncToken)', falling back to full sync")
				return false
			}
			syncState.lastSyncedAt = tokenDate

			try modelContext.save()
			logger.info("Incremental sync complete: \(changes.updated.count) updated, \(changes.deleted.count) deleted")
			return true
		} catch APIError.unauthorized {
			logger.error("Incremental sync: unauthorized")
			status = .error("Unauthorized")
			return false
		} catch {
			logger.warning("Incremental sync failed: \(error), falling back to full sync")
			return false
		}
	}

	// MARK: - On-Demand Content

	func fetchContentIfNeeded(documentId: String, modelContext: ModelContext) async throws -> CachedDocument? {
		guard let service else {
			throw SyncError.notConfigured
		}

		let descriptor = FetchDescriptor<CachedDocument>(
			predicate: #Predicate { $0.id == documentId }
		)
		guard let cached = try modelContext.fetch(descriptor).first else {
			return nil
		}

		let needsFetch = cached.contentBody == nil ||
			(cached.contentFetchedAt ?? .distantPast) < cached.lastSyncedAt

		if needsFetch {
			let doc = try await service.fetchDocument(vaultId: cached.vaultId, path: cached.path)
			cached.contentBody = doc.displayBody
			cached.title = doc.title
			cached.labels = doc.labels
			cached.docType = doc.docType
			cached.contentFetchedAt = .now
			try modelContext.save()
		}

		return cached
	}

	// MARK: - SSE Streaming

	func startSSEStream(vaultId: String, modelContext: ModelContext) {
		stopSSEStream()

		guard let service else {
			logger.warning("SyncEngine: no service configured, cannot start SSE stream")
			return
		}

		sseTask = Task {
			let baseURL = await service.baseURL
			let token = await service.token

			let url = baseURL.appendingPathComponent("events")
			guard var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
				logger.error("SSE: failed to create URL components from \(url)")
				return
			}
			components.queryItems = [URLQueryItem(name: "vaultId", value: vaultId)]

			guard let sseURL = components.url else {
				logger.error("SSE: failed to construct URL from components")
				return
			}

			var request = URLRequest(url: sseURL)
			request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
			request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
			request.timeoutInterval = 300

			var retryDelay: TimeInterval = 1

			while !Task.isCancelled {
				do {
					let (bytes, response) = try await URLSession.shared.bytes(for: request)

					guard let httpResponse = response as? HTTPURLResponse else {
						logger.warning("SSE: non-HTTP response, retrying...")
						try await Task.sleep(for: .seconds(retryDelay))
						retryDelay = min(retryDelay * 2, 60)
						continue
					}

					if httpResponse.statusCode == 401 {
						logger.error("SSE: unauthorized, stopping stream")
						status = .error("Unauthorized")
						break
					}

					guard httpResponse.statusCode == 200 else {
						logger.warning("SSE: HTTP \(httpResponse.statusCode), retrying...")
						try await Task.sleep(for: .seconds(retryDelay))
						retryDelay = min(retryDelay * 2, 60)
						continue
					}

					retryDelay = 1 // reset on success

					// Catch up on any events missed during the disconnect gap.
					// Try incremental sync first, fall back to full metadata sync.
					let synced = await performIncrementalSync(vaultId: vaultId, modelContext: modelContext)
					if !synced {
						await performMetadataSync(vaultId: vaultId, modelContext: modelContext)
					}

					var dataBuffer = ""

					for try await line in bytes.lines {
						if Task.isCancelled { break }

						if line.hasPrefix("data: ") {
							dataBuffer = String(line.dropFirst(6))
						} else if line.isEmpty, !dataBuffer.isEmpty {
							handleSSEEvent(data: dataBuffer, vaultId: vaultId, modelContext: modelContext)
							dataBuffer = ""
						}
					}
				} catch is CancellationError {
					break
				} catch let error as URLError where error.code == .cancelled {
					break
				} catch {
					if Task.isCancelled { break }
					logger.warning("SSE connection lost: \(error), reconnecting in \(retryDelay)s")
					do {
						try await Task.sleep(for: .seconds(retryDelay))
					} catch {
						break // Cancelled during backoff
					}
					retryDelay = min(retryDelay * 2, 60)
				}
			}
		}
	}

	func stopSSEStream() {
		sseTask?.cancel()
		sseTask = nil
	}

	// MARK: - Private Helpers

	private func handleSSEEvent(data: String, vaultId: String, modelContext: ModelContext) {
		guard let jsonData = data.data(using: .utf8) else {
			logger.warning("SSE: failed to convert event data to UTF-8")
			return
		}

		struct SSEEvent: Codable {
			let type: String
			let vaultId: String
			let payload: SSEPayload
		}

		struct SSEPayload: Codable {
			let docId: String?
			let path: String?
			let oldPath: String?
			let contentHash: String?
		}

		let event: SSEEvent
		do {
			event = try JSONDecoder().decode(SSEEvent.self, from: jsonData)
		} catch {
			logger.warning("SSE: failed to decode event: \(error)")
			return
		}

		do {
			switch event.type {
			case "file.created", "file.updated", "file.processed":
				guard let path = event.payload.path else {
					logger.warning("SSE: \(event.type) event missing path, skipping")
					return
				}
				let compositeId = CachedDocument.compositeId(vaultId: vaultId, path: path)
				let descriptor = FetchDescriptor<CachedDocument>(
					predicate: #Predicate { $0.id == compositeId }
				)
				if let existing = try modelContext.fetch(descriptor).first {
					existing.path = path
					existing.contentHash = event.payload.contentHash
					existing.lastSyncedAt = .now
					existing.invalidateContent()
				} else {
					let doc = CachedDocument(
						id: compositeId,
						vaultId: vaultId,
						path: path,
						title: CachedDocument.titleFromPath(path),
						contentHash: event.payload.contentHash,
						serverUpdatedAt: .now,
						lastSyncedAt: .now
					)
					modelContext.insert(doc)
				}

			case "file.deleted":
				guard let path = event.payload.path else {
					logger.warning("SSE: file.deleted event missing path, skipping")
					return
				}
				let compositeId = CachedDocument.compositeId(vaultId: vaultId, path: path)
				try deleteCachedDocument(compositeId: compositeId, modelContext: modelContext)

			case "file.moved":
				guard let path = event.payload.path, let oldPath = event.payload.oldPath else {
					logger.warning("SSE: file.moved event missing path or oldPath, skipping")
					return
				}
				let oldCompositeId = CachedDocument.compositeId(vaultId: vaultId, path: oldPath)
				let descriptor = FetchDescriptor<CachedDocument>(
					predicate: #Predicate { $0.id == oldCompositeId }
				)
				if let existing = try modelContext.fetch(descriptor).first {
					let moved = existing.moved(toPath: path)
					modelContext.delete(existing)
					modelContext.insert(moved)
				} else {
					logger.info("SSE: file.moved but \(oldPath) not in local cache, skipping")
				}

			default:
				logger.debug("SSE: ignoring event type \(event.type)")
			}

			try modelContext.save()
		} catch {
			logger.error("SSE: failed to process event \(event.type): \(error)")
		}
	}

	private func deleteCachedDocument(compositeId: String, modelContext: ModelContext) throws {
		let descriptor = FetchDescriptor<CachedDocument>(
			predicate: #Predicate { $0.id == compositeId }
		)
		if let existing = try modelContext.fetch(descriptor).first {
			modelContext.delete(existing)
		}
	}

	private func fetchOrCreateSyncState(vaultId: String, modelContext: ModelContext) throws -> SyncState {
		let descriptor = FetchDescriptor<SyncState>(
			predicate: #Predicate { $0.vaultId == vaultId }
		)
		if let existing = try modelContext.fetch(descriptor).first {
			return existing
		}
		let state = SyncState(vaultId: vaultId)
		modelContext.insert(state)
		return state
	}
}
