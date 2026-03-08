import Network
import SwiftUI

struct MainTabView: View {
    let service: KnowhowService

    @Environment(SyncEngine.self) private var syncEngine
    @State private var networkMonitor = NetworkMonitor()

    var body: some View {
        VStack(spacing: 0) {
            if !networkMonitor.isConnected {
                OfflineBanner()
            } else if case .error(let message) = syncEngine.status {
                SyncErrorBanner(message: message)
            }

            TabView {
                Tab("Browse", systemImage: "folder") {
                    NavigationStack {
                        VaultListView(service: service)
                    }
                }

                Tab("Search", systemImage: "magnifyingglass") {
                    NavigationStack {
                        SearchView(service: service)
                    }
                }
            }
        }
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
        monitor.start(queue: DispatchQueue(label: "NetworkMonitor"))
    }

    deinit {
        monitor.cancel()
    }
}
