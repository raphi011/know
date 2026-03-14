import Network
import SwiftData
import SwiftUI

struct MainSplitView: View {
    let service: KnowService

    @Environment(AuthService.self) private var authService
    @Environment(SyncEngine.self) private var syncEngine
    @Environment(\.modelContext) private var modelContext

    @State private var networkMonitor = NetworkMonitor()
    @State private var selectedDocumentId: String?
    @State private var selectedDocumentRef: DocumentReference?
    @State private var vaults: [Vault] = []
    @State private var isLoadingVaults = true
    @State private var vaultLoadError: String?
    @State private var columnVisibility = NavigationSplitViewVisibility.automatic

    // Search state
    @State private var searchQuery = ""
    @State private var isSearchPresented = false
    @State private var searchResults: [SearchResult] = []
    @State private var isSearching = false
    @State private var hasSearched = false
    @State private var searchError: String?
    @State private var searchTask: Task<Void, Never>?

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            sidebarContent
                .navigationTitle("Know")
                .searchable(text: $searchQuery, isPresented: $isSearchPresented, prompt: "Search documents...")
                .onChange(of: searchQuery) { _, newValue in
                    debounceSearch(newValue)
                }
                .toolbar {
                    ToolbarItem(placement: .navigationBarTrailing) {
                        Button("Logout", systemImage: "rectangle.portrait.and.arrow.right") {
                            authService.logout()
                        }
                    }
                }
                .task {
                    await loadVaults()
                }
        } detail: {
            detailContent
        }
        .navigationSplitViewColumnWidth(min: 280, ideal: 320, max: 400)
        .safeAreaInset(edge: .top) {
            if !networkMonitor.isConnected {
                OfflineBanner()
            } else if case .error(let message) = syncEngine.status {
                SyncErrorBanner(message: message)
            }
        }
    }

    // MARK: - Sidebar

    @ViewBuilder
    private var sidebarContent: some View {
        if isSearchPresented && !searchQuery.isEmpty {
            searchResultsList
        } else {
            vaultTreeList
        }
    }

    @ViewBuilder
    private var vaultTreeList: some View {
        if isLoadingVaults {
            ProgressView()
        } else if let vaultLoadError {
            ContentUnavailableView {
                Label("Error", systemImage: "exclamationmark.triangle")
            } description: {
                Text(vaultLoadError)
            } actions: {
                Button("Retry") {
                    Task { await loadVaults() }
                }
            }
        } else if vaults.isEmpty {
            ContentUnavailableView("No Vaults", systemImage: "tray")
        } else {
            List(selection: $selectedDocumentId) {
                ForEach(vaults) { vault in
                    SidebarVaultSection(
                        service: service,
                        vault: vault,
                        selectedDocumentId: $selectedDocumentId,
                        selectedDocumentRef: $selectedDocumentRef
                    )
                }
            }
            .refreshable {
                await loadVaults()
            }
        }
    }

    @ViewBuilder
    private var searchResultsList: some View {
        List(selection: $selectedDocumentId) {
            if isSearching {
                HStack {
                    Spacer()
                    ProgressView()
                    Spacer()
                }
            } else if let searchError {
                Label(searchError, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
            } else if hasSearched && searchResults.isEmpty {
                ContentUnavailableView.search(text: searchQuery)
            } else {
                ForEach(searchResults) { result in
                    SearchResultRow(result: result)
                        .tag(result.documentId)
                }
            }
        }
    }

    // MARK: - Detail

    @ViewBuilder
    private var detailContent: some View {
        if let ref = selectedDocumentRef {
            DocumentView(service: service, reference: ref)
                .id(selectedDocumentId)
        } else {
            ContentUnavailableView("Select a Document", systemImage: "doc.text")
        }
    }

    // MARK: - Data Loading

    private func loadVaults() async {
        let showSpinner = vaults.isEmpty
        if showSpinner { isLoadingVaults = true }
        vaultLoadError = nil
        defer { if showSpinner { isLoadingVaults = false } }

        do {
            vaults = try await service.fetchVaults()
        } catch is CancellationError {
            return
        } catch {
            vaultLoadError = error.localizedDescription
        }
    }

    // MARK: - Search

    private func debounceSearch(_ text: String) {
        searchTask?.cancel()

        guard !text.trimmingCharacters(in: .whitespaces).isEmpty else {
            searchResults = []
            hasSearched = false
            searchError = nil
            return
        }

        searchTask = Task {
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            await performSearch(text)
        }
    }

    private func performSearch(_ text: String) async {
        guard let vault = vaults.first else {
            searchError = "No vaults available"
            hasSearched = true
            return
        }

        isSearching = true
        searchError = nil
        defer { isSearching = false }

        do {
            searchResults = try await service.search(vaultId: vault.id, query: text)
            hasSearched = true
        } catch is CancellationError {
            return
        } catch {
            searchError = error.localizedDescription
            hasSearched = true
        }
    }
}

// MARK: - Sidebar Vault Section

private struct SidebarVaultSection: View {
    let service: KnowService
    let vault: Vault

    @Binding var selectedDocumentId: String?
    @Binding var selectedDocumentRef: DocumentReference?

    @Environment(SyncEngine.self) private var syncEngine
    @Environment(\.modelContext) private var modelContext
    @Query private var allDocuments: [CachedDocument]
    @Query private var syncStates: [SyncState]

    init(
        service: KnowService,
        vault: Vault,
        selectedDocumentId: Binding<String?>,
        selectedDocumentRef: Binding<DocumentReference?>
    ) {
        self.service = service
        self.vault = vault
        _selectedDocumentId = selectedDocumentId
        _selectedDocumentRef = selectedDocumentRef

        let vid = vault.id
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

    var body: some View {
        Section(vault.name) {
            if !isInitialSyncComplete {
                ProgressView("Syncing...")
            } else {
                SidebarFolderTree(
                    documents: allDocuments,
                    folder: "/",
                    selectedDocumentId: $selectedDocumentId,
                    selectedDocumentRef: $selectedDocumentRef
                )
            }
        }
        .task {
            await syncEngine.performMetadataSync(vaultId: vault.id, modelContext: modelContext)
            syncEngine.startSSEStream(vaultId: vault.id, modelContext: modelContext)
        }
        .onChange(of: selectedDocumentId) { _, newId in
            guard let newId else {
                selectedDocumentRef = nil
                return
            }
            if let doc = allDocuments.first(where: { $0.id == newId }) {
                selectedDocumentRef = .cached(doc)
            } else {
                // Selected from search results — resolve by ID
                selectedDocumentRef = .byId(newId)
            }
        }
    }
}

// MARK: - Sidebar Folder Tree (Recursive)

private struct SidebarFolderTree: View {
    let documents: [CachedDocument]
    let folder: String

    @Binding var selectedDocumentId: String?
    @Binding var selectedDocumentRef: DocumentReference?

    private var documentsInFolder: [CachedDocument] {
        documents.filter { parentFolder(of: $0.path) == folder }
    }

    private var subfolders: [(name: String, path: String)] {
        let prefix = folder.hasSuffix("/") ? folder : folder + "/"
        var seen = Set<String>()
        var result: [(name: String, path: String)] = []

        for doc in documents {
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
        ForEach(subfolders, id: \.path) { subfolder in
            DisclosureGroup {
                SidebarFolderTree(
                    documents: documents,
                    folder: subfolder.path,
                    selectedDocumentId: $selectedDocumentId,
                    selectedDocumentRef: $selectedDocumentRef
                )
            } label: {
                Label(subfolder.name, systemImage: "folder")
            }
        }

        ForEach(documentsInFolder) { doc in
            Label(doc.title, systemImage: "doc.text")
                .tag(doc.id)
                .lineLimit(1)
        }
    }

    private func parentFolder(of path: String) -> String {
        guard let lastSlash = path.lastIndex(of: "/") else { return "/" }
        let parent = String(path[..<lastSlash])
        return parent.isEmpty ? "/" : parent
    }
}

// MARK: - Offline Banner

private struct OfflineBanner: View {
    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "wifi.slash")
                .font(.caption)
            Text("Offline — showing cached data")
                .font(.caption)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 6)
        .background(.orange.opacity(0.15))
        .foregroundStyle(.orange)
    }
}

private struct SyncErrorBanner: View {
    let message: String

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.triangle")
                .font(.caption)
            Text("Sync error: \(message)")
                .font(.caption)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 6)
        .background(.red.opacity(0.1))
        .foregroundStyle(.red)
    }
}

// MARK: - Network Monitor

@MainActor
@Observable
final class NetworkMonitor {
    private(set) var isConnected = true
    private let monitor = NWPathMonitor()

    init() {
        monitor.pathUpdateHandler = { [weak self] path in
            let connected = path.status == .satisfied
            Task { @MainActor in
                self?.isConnected = connected
            }
        }
        let queue = DispatchQueue(label: "NetworkMonitor")
        monitor.start(queue: queue)
        // Seed initial state synchronously from current path
        isConnected = monitor.currentPath.status == .satisfied
    }

    deinit {
        monitor.cancel()
    }
}
