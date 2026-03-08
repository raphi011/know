import SwiftUI

struct VaultListView: View {
    let service: KnowhowService
    @Environment(AuthService.self) private var authService
    @State private var vaults: [Vault] = []
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
                        Task { await loadVaults() }
                    }
                }
            } else if vaults.isEmpty {
                ContentUnavailableView("No Vaults", systemImage: "tray")
            } else {
                List(vaults) { vault in
                    NavigationLink(value: vault) {
                        VaultRow(vault: vault)
                    }
                }
            }
        }
        .navigationTitle("Vaults")
        .navigationDestination(for: Vault.self) { vault in
            FolderBrowserView(service: service, vaultId: vault.id, vaultName: vault.name, folder: nil)
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
    }

    private func loadVaults() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            vaults = try await service.fetchVaults()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

private struct VaultRow: View {
    let vault: Vault

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(vault.name)
                .font(.headline)
            if let description = vault.description, !description.isEmpty {
                Text(description)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            if !vault.labels.isEmpty {
                Text(vault.labels.joined(separator: ", "))
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.vertical, 2)
    }
}
