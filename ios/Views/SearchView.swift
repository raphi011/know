import SwiftUI

struct SearchView: View {
    let service: KnowhowService

    @State private var query = ""
    @State private var selectedVault: Vault?
    @State private var vaults: [Vault] = []
    @State private var results: [SearchResult] = []
    @State private var isSearching = false
    @State private var hasSearched = false
    @State private var vaultLoadError: String?
    @State private var searchError: String?
    @State private var searchTask: Task<Void, Never>?

    var body: some View {
        List {
            if let vaultLoadError {
                Section {
                    Label(vaultLoadError, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.red)
                }
            }

            if vaults.count > 1 {
                Section {
                    Picker("Vault", selection: $selectedVault) {
                        ForEach(vaults) { vault in
                            Text(vault.name).tag(Optional(vault))
                        }
                    }
                }
            }

            if isSearching {
                Section {
                    HStack {
                        Spacer()
                        ProgressView()
                        Spacer()
                    }
                }
            } else if let searchError {
                Section {
                    Label(searchError, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.red)
                }
            } else if hasSearched && results.isEmpty {
                ContentUnavailableView.search(text: query)
            } else if !results.isEmpty {
                Section("Results (\(results.count))") {
                    ForEach(results) { result in
                        NavigationLink {
                            DocumentView(
                                service: service,
                                reference: .byId(result.documentId)
                            )
                        } label: {
                            SearchResultRow(result: result)
                        }
                    }
                }
            }
        }
        .navigationTitle("Search")
        .searchable(text: $query, prompt: "Search documents...")
        .onChange(of: query) { _, newValue in
            debounceSearch(newValue)
        }
        .task {
            do {
                vaults = try await service.fetchVaults()
                selectedVault = vaults.first
            } catch {
                vaultLoadError = error.localizedDescription
            }
        }
    }

    private func debounceSearch(_ text: String) {
        searchTask?.cancel()

        guard !text.trimmingCharacters(in: .whitespaces).isEmpty else {
            results = []
            hasSearched = false
            return
        }

        searchTask = Task {
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            await performSearch(text)
        }
    }

    private func performSearch(_ text: String) async {
        guard let vault = selectedVault else { return }

        isSearching = true
        searchError = nil
        defer { isSearching = false }

        do {
            results = try await service.search(vaultId: vault.id, query: text)
            hasSearched = true
        } catch is CancellationError {
            return
        } catch {
            searchError = error.localizedDescription
            hasSearched = true
        }
    }
}
