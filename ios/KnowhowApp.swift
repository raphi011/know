import SwiftUI

@main
struct KnowhowApp: App {
    @State private var authService = AuthService()
    @State private var service: KnowhowService?
    @State private var hasAttemptedRestore = false
    @State private var restoreError: Error?

    var body: some Scene {
        WindowGroup {
            Group {
                if !hasAttemptedRestore {
                    ProgressView("Connecting...")
                        .task {
                            restoreError = await authService.restoreSession()
                            if let client = authService.client {
                                service = KnowhowService(client: client)
                            }
                            hasAttemptedRestore = true
                        }
                } else if let service {
                    MainTabView(service: service)
                        .environment(authService)
                } else {
                    LoginView(restoreError: restoreError)
                        .environment(authService)
                }
            }
            .onChange(of: authService.isAuthenticated) {
                if let client = authService.client {
                    service = KnowhowService(client: client)
                    restoreError = nil
                } else {
                    service = nil
                    restoreError = nil
                }
            }
        }
    }
}
