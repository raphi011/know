import Foundation
import SwiftData

@Model
final class CachedDocument {
    @Attribute(.unique) var id: String
    var vaultId: String
    var path: String
    var title: String
    var contentBody: String?
    var labels: [String]
    var docType: String?
    var source: String
    var contentHash: String?
    var serverUpdatedAt: Date
    var lastSyncedAt: Date
    var contentFetchedAt: Date?

    init(
        id: String,
        vaultId: String,
        path: String,
        title: String,
        contentBody: String? = nil,
        labels: [String] = [],
        docType: String? = nil,
        source: String = "manual",
        contentHash: String? = nil,
        serverUpdatedAt: Date = .now,
        lastSyncedAt: Date = .now,
        contentFetchedAt: Date? = nil
    ) {
        self.id = id
        self.vaultId = vaultId
        self.path = path
        self.title = title
        self.contentBody = contentBody
        self.labels = labels
        self.docType = docType
        self.source = source
        self.contentHash = contentHash
        self.serverUpdatedAt = serverUpdatedAt
        self.lastSyncedAt = lastSyncedAt
        self.contentFetchedAt = contentFetchedAt
    }

    func invalidateContent() {
        contentBody = nil
        contentFetchedAt = nil
    }

    static func titleFromPath(_ path: String) -> String {
        path.components(separatedBy: "/").last ?? path
    }
}

@Model
final class SyncState {
    @Attribute(.unique) var vaultId: String
    var lastSyncedAt: Date?
    var isInitialSyncComplete: Bool

    init(vaultId: String, lastSyncedAt: Date? = nil, isInitialSyncComplete: Bool = false) {
        self.vaultId = vaultId
        self.lastSyncedAt = lastSyncedAt
        self.isInitialSyncComplete = isInitialSyncComplete
    }
}
