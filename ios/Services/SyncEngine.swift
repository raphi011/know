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

    private(set) var status: Status = .idle
    private(set) var service: KnowService?
    private var sseTask: Task<Void, Never>?

    func configure(service: KnowService?) {
        self.service = service
    }

    // MARK: - Metadata Sync

    func performMetadataSync(vaultId: String, modelContext: ModelContext) async {
        guard let service else {
            logger.warning("SyncEngine: no service configured")
            return
        }

        status = .syncing("Syncing metadata...")
        defer { if case .syncing = status { status = .idle } }

        do {
            let syncState = try fetchOrCreateSyncState(vaultId: vaultId, modelContext: modelContext)
            var offset = 0
            let pageSize = 500
            var latestUpdatedAt: Date?

            while true {
                let result = try await service.fetchSyncMetadata(
                    vaultId: vaultId,
                    since: syncState.lastSyncedAt,
                    limit: pageSize,
                    offset: offset
                )

                for meta in result.documents {
                    try upsertCachedDocument(from: meta, vaultId: vaultId, modelContext: modelContext)
                    if let date = parseDate(meta.updatedAt) {
                        if latestUpdatedAt == nil || date > latestUpdatedAt! {
                            latestUpdatedAt = date
                        }
                    }
                }

                for tombstone in result.tombstones {
                    try deleteCachedDocument(docId: tombstone.docId, modelContext: modelContext)
                }

                if !result.hasMore { break }
                offset += pageSize
            }

            if let latestUpdatedAt {
                syncState.lastSyncedAt = latestUpdatedAt
            }
            syncState.isInitialSyncComplete = true
            try modelContext.save()

            logger.info("Metadata sync complete for vault \(vaultId)")
        } catch {
            logger.error("Metadata sync failed: \(error)")
            status = .error(error.localizedDescription)
        }
    }

    // MARK: - On-Demand Content

    func fetchContentIfNeeded(documentId: String, modelContext: ModelContext) async throws -> CachedDocument? {
        guard let service else {
            logger.warning("SyncEngine: no service configured, cannot fetch content")
            return nil
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
            let doc = try await service.fetchDocumentById(id: documentId)
            cached.contentBody = doc.contentBody
            cached.title = doc.title
            cached.labels = doc.labels
            cached.docType = doc.docType
            cached.source = doc.source
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
            let url = service.baseURL.appendingPathComponent("events")
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
            request.setValue("Bearer \(service.token)", forHTTPHeaderField: "Authorization")
            request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
            request.timeoutInterval = 300

            while !Task.isCancelled {
                do {
                    let (bytes, response) = try await URLSession.shared.bytes(for: request)

                    guard let httpResponse = response as? HTTPURLResponse,
                          httpResponse.statusCode == 200 else {
                        logger.warning("SSE: non-200 response, retrying in 5s")
                        try await Task.sleep(for: .seconds(5))
                        continue
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
                } catch {
                    if Task.isCancelled { break }
                    logger.warning("SSE connection lost: \(error), reconnecting in 5s")
                    try? await Task.sleep(for: .seconds(5))
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
            case "document.created", "document.updated":
                guard let docId = event.payload.docId, let path = event.payload.path else {
                    logger.warning("SSE: \(event.type) event missing docId or path, skipping")
                    return
                }
                let descriptor = FetchDescriptor<CachedDocument>(
                    predicate: #Predicate { $0.id == docId }
                )
                if let existing = try modelContext.fetch(descriptor).first {
                    existing.path = path
                    existing.contentHash = event.payload.contentHash
                    existing.lastSyncedAt = .now
                    existing.invalidateContent()
                } else {
                    let doc = CachedDocument(
                        id: docId,
                        vaultId: vaultId,
                        path: path,
                        title: CachedDocument.titleFromPath(path),
                        contentHash: event.payload.contentHash,
                        serverUpdatedAt: .now,
                        lastSyncedAt: .now
                    )
                    modelContext.insert(doc)
                }

            case "document.deleted":
                guard let docId = event.payload.docId else {
                    logger.warning("SSE: document.deleted event missing docId, skipping")
                    return
                }
                try deleteCachedDocument(docId: docId, modelContext: modelContext)

            case "document.moved":
                guard let docId = event.payload.docId, let path = event.payload.path else {
                    logger.warning("SSE: document.moved event missing docId or path, skipping")
                    return
                }
                let descriptor = FetchDescriptor<CachedDocument>(
                    predicate: #Predicate { $0.id == docId }
                )
                if let existing = try modelContext.fetch(descriptor).first {
                    existing.path = path
                    existing.lastSyncedAt = .now
                }

            default:
                logger.debug("SSE: ignoring event type \(event.type)")
            }

            try modelContext.save()
        } catch {
            logger.error("SSE: failed to process event \(event.type): \(error)")
        }
    }

    private func upsertCachedDocument(from meta: SyncMetaItem, vaultId: String, modelContext: ModelContext) throws {
        let metaId = meta.id
        let descriptor = FetchDescriptor<CachedDocument>(
            predicate: #Predicate { $0.id == metaId }
        )

        if let existing = try modelContext.fetch(descriptor).first {
            existing.path = meta.path
            existing.lastSyncedAt = .now
            if let date = parseDate(meta.updatedAt) {
                existing.serverUpdatedAt = date
            }
            if existing.contentHash != meta.contentHash {
                existing.contentHash = meta.contentHash
                existing.invalidateContent()
            }
        } else {
            let doc = CachedDocument(
                id: meta.id,
                vaultId: vaultId,
                path: meta.path,
                title: CachedDocument.titleFromPath(meta.path),
                contentHash: meta.contentHash,
                serverUpdatedAt: parseDate(meta.updatedAt) ?? .now,
                lastSyncedAt: .now
            )
            modelContext.insert(doc)
        }
    }

    private func deleteCachedDocument(docId: String, modelContext: ModelContext) throws {
        let descriptor = FetchDescriptor<CachedDocument>(
            predicate: #Predicate { $0.id == docId }
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

    private func parseDate(_ string: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        guard let date = formatter.date(from: string) else {
            logger.warning("Failed to parse date: \(string)")
            return nil
        }
        return date
    }
}
