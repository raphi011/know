import SwiftUI

struct LoginView: View {
    @Environment(AuthService.self) private var authService
    var restoreError: Error?
    @State private var serverURL: String = ""
    @State private var token: String = ""
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Server URL", text: $serverURL)
                        .textContentType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)

                    SecureField("API Token", text: $token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("Connection")
                } footer: {
                    Text("Enter your Knowhow server URL and API token (starts with kh_).")
                }

                if let displayError = errorMessage ?? restoreError?.localizedDescription {
                    Section {
                        Label(displayError, systemImage: "exclamationmark.triangle")
                            .foregroundStyle(.red)
                    }
                }

                Section {
                    Button {
                        Task { await connect() }
                    } label: {
                        if isLoading {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                        } else {
                            Text("Connect")
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .disabled(serverURL.isEmpty || token.isEmpty || isLoading)
                }
            }
            .navigationTitle("Knowhow")
            .onAppear {
                let storedURL = authService.serverURL
                if !storedURL.isEmpty {
                    serverURL = storedURL
                }
            }
        }
    }

    private func connect() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            try await authService.login(serverURL: serverURL, token: token)
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
