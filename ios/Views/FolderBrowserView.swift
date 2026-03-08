import SwiftData
import SwiftUI

struct FolderBrowserView: View {
    let service: KnowhowService
    let vaultId: String
    let vaultName: String
    let folder: String?

    @Environment(SyncEngine.self) private var syncEngine
    @Environment(\.modelContext) private var modelContext
    @Query private var allDocuments: [CachedDocument]
    @Query private var syncStates: [SyncState]

    init(service: KnowhowService, vaultId: String, vaultName: String, folder: String?) {
        self.service = service
        self.vaultId = vaultId
        self.vaultName = vaultName
        self.folder = folder

        let vid = vaultId
        _allDocuments = Query(
            filter: #Predicate<CachedDocument> { $0.vaultId == vid },
            sort: [SortDescriptor(\.path)]
        )
        _syncStates = Query(
            filter: #Predicate<SyncState> { $0.vaultId == vid }
        )
    }

    private var isInitialSyncComplete: Bool {
        syncStates.first?.isInitialSyncComplete ?? false
    }

    /// Documents directly in this folder (not in subfolders).
    private var documentsInFolder: [CachedDocument] {
        let prefix = folder ?? "/"
        return allDocuments.filter { doc in
            let docFolder = parentFolder(of: doc.path)
            return docFolder == prefix
        }
    }

    /// Subfolders derived from document paths.
    private var subfolders: [(name: String, path: String)] {
        let prefix = (folder ?? "/").hasSuffix("/") ? (folder ?? "/") : (folder ?? "/") + "/"
        var seen = Set<String>()
        var result: [(name: String, path: String)] = []

        for doc in allDocuments {
            guard doc.path.hasPrefix(prefix) else { continue }
            let remainder = String(doc.path.dropFirst(prefix.count))
            guard let slashIndex = remainder.firstIndex(of: "/") else { continue }
            let folderName = String(remainder[..<slashIndex])
            let folderPath = prefix + folderName
            if seen.insert(folderPath).inserted {
                result.append((name: folderName, path: folderPath))
            }
        }

        return result.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    var body: some View {
        Group {
            if !isInitialSyncComplete {
                ProgressView("Syncing...")
            } else if subfolders.isEmpty && documentsInFolder.isEmpty {
                ContentUnavailableView("Empty Folder", systemImage: "folder")
            } else {
                List {
                    if !subfolders.isEmpty {
                        Section("Folders") {
                            ForEach(subfolders, id: \.path) { subfolder in
                                NavigationLink {
                                    FolderBrowserView(
                                        service: service,
                                        vaultId: vaultId,
                                        vaultName: vaultName,
                                        folder: subfolder.path
                                    )
                                } label: {
                                    Label(subfolder.name, systemImage: "folder")
                                }
                            }
                        }
                    }

                    if !documentsInFolder.isEmpty {
                        Section("Documents") {
                            ForEach(documentsInFolder) { doc in
                                NavigationLink {
                                    DocumentView(
                                        service: service,
                                        reference: .cached(doc)
                                    )
                                } label: {
                                    CachedDocumentRow(document: doc)
                                }
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle(folder?.components(separatedBy: "/").last ?? vaultName)
        .task {
            // Only trigger sync at the vault root, not on every subfolder navigation
            guard folder == nil else { return }
            await syncEngine.performMetadataSync(vaultId: vaultId, modelContext: modelContext)
            syncEngine.startSSEStream(vaultId: vaultId, modelContext: modelContext)
        }
    }

    private func parentFolder(of path: String) -> String {
        guard let lastSlash = path.lastIndex(of: "/") else { return "/" }
        let parent = String(path[..<lastSlash])
        return parent.isEmpty ? "/" : parent
    }
}

/// Row view for a cached document.
struct CachedDocumentRow: View {
    let document: CachedDocument

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(document.title)
                .font(.body)
                .lineLimit(1)

            HStack(spacing: 8) {
                if let docType = document.docType {
                    Text(docType)
                        .font(.caption)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(.fill.tertiary)
                        .clipShape(Capsule())
                }

                if !document.labels.isEmpty {
                    Text(document.labels.joined(separator: ", "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 2)
    }
}
