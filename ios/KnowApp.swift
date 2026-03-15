import SwiftData
import SwiftUI

@main
struct KnowApp: App {
    @State private var authService = AuthService()
    @State private var service: KnowService?
    @State private var syncEngine = SyncEngine()
    @State private var hasAttemptedRestore = false
    @State private var restoreError: Error?

    let modelContainer: ModelContainer

    init() {
        do {
            modelContainer = try ModelContainer(for: CachedDocument.self, SyncState.self, RecentDocument.self)
        } catch {
            fatalError("Failed to create ModelContainer: \(error)")
        }
    }

    var body: some Scene {
        WindowGroup {
            Group {
                if !hasAttemptedRestore {
                    ProgressView("Connecting...")
                        .task {
                            restoreError = await authService.restoreSession()
                            if let client = authService.client {
                                let newService = KnowService(client: client)
                                service = newService
                                syncEngine.configure(service: newService)
                            }
                            hasAttemptedRestore = true
                        }
                } else if let service {
                    MainSplitView(service: service)
                        .environment(authService)
                        .environment(syncEngine)
                } else {
                    LoginView(restoreError: restoreError)
                        .environment(authService)
                }
            }
            .onChange(of: authService.isAuthenticated) {
                if let client = authService.client {
                    let newService = KnowService(client: client)
                    service = newService
                    syncEngine.configure(service: newService)
                    restoreError = nil
                } else {
                    service = nil
                    syncEngine.configure(service: nil)
                    syncEngine.stopSSEStream()
                    restoreError = nil
                }
            }
        }
        .modelContainer(modelContainer)
        #if os(macOS)
        .defaultSize(width: 900, height: 600)
        #endif
    }
}
