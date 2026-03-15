import Foundation
import OSLog
import SwiftData

private let logger = Logger(subsystem: "Know", category: "RecentDocument")

@Model
final class RecentDocument {
    static let maxEntries = 50

    @Attribute(.unique) var compositeId: String
    var vaultId: String
    var path: String
    var title: String
    var accessedAt: Date

    init(vaultId: String, path: String, title: String, accessedAt: Date = .now) {
        self.compositeId = Self.compositeId(vaultId: vaultId, path: path)
        self.vaultId = vaultId
        self.path = path
        self.title = title
        self.accessedAt = accessedAt
    }

    static func compositeId(vaultId: String, path: String) -> String {
        "\(vaultId):\(path)"
    }

    /// Records a document access, creating or updating the recent entry and pruning old entries.
    @MainActor
    static func recordAccess(
        vaultId: String,
        path: String,
        title: String,
        modelContext: ModelContext
    ) {
        let id = compositeId(vaultId: vaultId, path: path)

        let predicate = #Predicate<RecentDocument> { $0.compositeId == id }
        let descriptor = FetchDescriptor(predicate: predicate)

        do {
            if let existing = try modelContext.fetch(descriptor).first {
                existing.accessedAt = .now
                existing.title = title
            } else {
                let entry = RecentDocument(
                    vaultId: vaultId,
                    path: path,
                    title: title
                )
                modelContext.insert(entry)
            }
        } catch {
            logger.error("Failed to record access for \(path): \(error)")
            return
        }

        // Prune beyond maxEntries
        let allDescriptor = FetchDescriptor<RecentDocument>(
            sortBy: [SortDescriptor(\.accessedAt, order: .reverse)]
        )
        do {
            let all = try modelContext.fetch(allDescriptor)
            if all.count > maxEntries {
                for entry in all.dropFirst(maxEntries) {
                    modelContext.delete(entry)
                }
            }
        } catch {
            logger.error("Failed to prune recent documents: \(error)")
        }
    }
}
