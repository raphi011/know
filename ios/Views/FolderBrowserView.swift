import SwiftUI

struct FolderBrowserView: View {
    let service: KnowhowService
    let vaultId: String
    let vaultName: String
    let folder: String?

    @State private var folders: [Folder] = []
    @State private var documents: [DocumentSummary] = []
    @State private var isLoading = true
    @State private var errorMessage: String?

    var body: some View {
        Group {
            if isLoading {
                ProgressView()
            } else if let errorMessage {
                ContentUnavailableView {
                    Label("Error", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(errorMessage)
                } actions: {
                    Button("Retry") {
                        Task { await loadContents() }
                    }
                }
            } else if folders.isEmpty && documents.isEmpty {
                ContentUnavailableView("Empty Folder", systemImage: "folder")
            } else {
                List {
                    if !folders.isEmpty {
                        Section("Folders") {
                            ForEach(folders) { subfolder in
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

                    if !documents.isEmpty {
                        Section("Documents") {
                            ForEach(documents) { doc in
                                NavigationLink {
                                    DocumentView(
                                        service: service,
                                        reference: .byPath(vaultId: vaultId, path: doc.path)
                                    )
                                } label: {
                                    DocumentRow(document: doc)
                                }
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle(folder?.components(separatedBy: "/").last ?? vaultName)
        .refreshable {
            await loadContents()
        }
        .task {
            await loadContents()
        }
    }

    private func loadContents() async {
        // Only show full-screen spinner on initial load, not on pull-to-refresh
        let showSpinner = folders.isEmpty && documents.isEmpty
        if showSpinner { isLoading = true }
        errorMessage = nil
        defer { if showSpinner { isLoading = false } }

        do {
            let vault = try await service.fetchVault(id: vaultId, folder: folder)
            folders = vault.folders ?? []
            documents = vault.documents ?? []
        } catch is CancellationError {
            // Task cancelled (e.g. view disappeared) — ignore silently
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
