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
		self.contentHash = contentHash
		self.serverUpdatedAt = serverUpdatedAt
		self.lastSyncedAt = lastSyncedAt
		self.contentFetchedAt = contentFetchedAt
	}

	static func compositeId(vaultId: String, path: String) -> String {
		"\(vaultId):\(path)"
	}

	/// Extracts the path component from a composite ID ("vaultId:path" -> "path").
	static func pathFromCompositeId(_ compositeId: String) -> String? {
		guard let colonIndex = compositeId.firstIndex(of: ":") else { return nil }
		return String(compositeId[compositeId.index(after: colonIndex)...])
	}

	func invalidateContent() {
		contentBody = nil
		contentFetchedAt = nil
	}

	/// Creates a new CachedDocument with the updated path.
	/// SwiftData @Attribute(.unique) IDs cannot be mutated in place — delete the old record and insert the returned one.
	func moved(toPath newPath: String) -> CachedDocument {
		CachedDocument(
			id: Self.compositeId(vaultId: vaultId, path: newPath),
			vaultId: vaultId,
			path: newPath,
			title: Self.titleFromPath(newPath),
			contentBody: contentBody,
			labels: labels,
			docType: docType,
			contentHash: contentHash,
			serverUpdatedAt: .now,
			lastSyncedAt: .now,
			contentFetchedAt: contentFetchedAt
		)
	}

	static func titleFromPath(_ path: String) -> String {
		let filename = path.components(separatedBy: "/").last ?? path
		if filename.hasSuffix(".md") {
			return String(filename.dropLast(3))
		}
		return filename
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
