import Foundation
import OSLog
import SwiftData

private let logger = Logger(subsystem: "Know", category: "QuickPicker")

struct QuickPickerItem: Identifiable {
    let path: String
    let title: String
    let labels: [String]
    let docType: String?
    let score: Int
    let matchedIndices: [Int]
    let isRecent: Bool

    var id: String { path }
}

@MainActor
@Observable
final class QuickPickerViewModel {
    var query = "" {
        didSet { updateResults() }
    }
    private(set) var results: [QuickPickerItem] = []
    private(set) var selectedIndex = 0
    private(set) var loadError: String?

    private var allDocuments: [CachedDocument] = []
    private var recentDocuments: [RecentDocument] = []

    private var normalizedQuery: String? {
        let trimmed = query.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }
        return trimmed.hasPrefix("/") ? trimmed : "/\(trimmed)"
    }

    var canCreate: Bool {
        guard let path = normalizedQuery else { return false }
        let withMd = path.hasSuffix(".md") ? path : "\(path).md"
        return !allDocuments.contains { $0.path == path || $0.path == withMd }
    }

    var createPath: String {
        guard let path = normalizedQuery else { return "" }
        return path.hasSuffix(".md") ? path : "\(path).md"
    }

    func load(vaultId: String, modelContext: ModelContext) {
        loadError = nil

        let docPredicate = #Predicate<CachedDocument> { $0.vaultId == vaultId }
        let docDescriptor = FetchDescriptor(
            predicate: docPredicate,
            sortBy: [SortDescriptor(\.path)]
        )

        let recentPredicate = #Predicate<RecentDocument> { $0.vaultId == vaultId }
        let recentDescriptor = FetchDescriptor(
            predicate: recentPredicate,
            sortBy: [SortDescriptor(\.accessedAt, order: .reverse)]
        )

        do {
            allDocuments = try modelContext.fetch(docDescriptor)
            recentDocuments = try modelContext.fetch(recentDescriptor)
        } catch {
            logger.warning("Failed to load documents: \(error)")
            loadError = error.localizedDescription
            allDocuments = []
            recentDocuments = []
        }
        updateResults()
    }

    func moveSelection(by offset: Int) {
        let maxIndex = results.count + (canCreate ? 1 : 0) - 1
        guard maxIndex >= 0 else { return }
        selectedIndex = min(max(selectedIndex + offset, 0), maxIndex)
    }

    func selectedItem() -> QuickPickerItem? {
        guard selectedIndex >= 0 && selectedIndex < results.count else { return nil }
        return results[selectedIndex]
    }

    var isCreateSelected: Bool {
        canCreate && selectedIndex == results.count
    }

    private func updateResults() {
        selectedIndex = 0

        let trimmed = query.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else {
            // Show recents, filtered to documents that still exist
            let docPaths = Set(allDocuments.map(\.path))
            results = recentDocuments.compactMap { recent in
                guard docPaths.contains(recent.path) else { return nil }
                return QuickPickerItem(
                    path: recent.path,
                    title: recent.title,
                    labels: [],
                    docType: nil,
                    score: 0,
                    matchedIndices: [],
                    isRecent: true
                )
            }
            return
        }

        results = allDocuments.compactMap { doc in
            guard let match = fuzzyMatch(query: trimmed, target: doc.path) else {
                return nil
            }
            return QuickPickerItem(
                path: doc.path,
                title: doc.title,
                labels: doc.labels,
                docType: doc.docType,
                score: match.score,
                matchedIndices: match.matchedIndices,
                isRecent: false
            )
        }
        .sorted { $0.score > $1.score }
        .prefix(50)
        .map { $0 }
    }
}
