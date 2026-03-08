import SwiftUI

struct MainTabView: View {
    let service: KnowhowService

    var body: some View {
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
