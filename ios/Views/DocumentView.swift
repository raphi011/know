import MarkdownUI
import SwiftUI

enum DocumentReference {
    case byPath(vaultId: String, path: String)
    case byId(String)
    case cached(CachedDocument)
}

struct DocumentView: View {
    let service: KnowhowService
    let reference: DocumentReference

    @Environment(SyncEngine.self) private var syncEngine
    @Environment(\.modelContext) private var modelContext

    @State private var document: Document?
    @State private var cachedDoc: CachedDocument?
    @State private var isLoading = true
    @State private var errorMessage: String?

    private var displayTitle: String {
        document?.title ?? cachedDoc?.title ?? "Document"
    }

    private var displayContentBody: String? {
        document?.contentBody ?? cachedDoc?.contentBody
    }

    var body: some View {
        Group {
            if isLoading {
                ProgressView()
            } else if let errorMessage {
                ContentUnavailableView {
                    Label("Error", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(errorMessage)
                }
            } else if let contentBody = displayContentBody {
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        let docType = document?.docType ?? cachedDoc?.docType
                        let labels = document?.labels ?? cachedDoc?.labels ?? []

                        if docType != nil || !labels.isEmpty {
                            HStack(spacing: 6) {
                                if let docType {
                                    Text(docType)
                                        .font(.caption)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 3)
                                        .background(.fill.tertiary)
                                        .clipShape(Capsule())
                                }

                                ForEach(labels, id: \.self) { label in
                                    Text(label)
                                        .font(.caption)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 3)
                                        .background(.blue.opacity(0.1))
                                        .foregroundStyle(.blue)
                                        .clipShape(Capsule())
                                }
                            }
                        }

                        Markdown(contentBody)
                            .textSelection(.enabled)
                    }
                    .padding()
                }
            } else {
                ContentUnavailableView("No Content", systemImage: "doc")
            }
        }
        .navigationTitle(displayTitle)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await loadDocument()
        }
    }

    private func loadDocument() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        switch reference {
        case .cached(let cached):
            cachedDoc = cached
            if cached.contentBody != nil {
                return
            }
            // Fetch content on demand
            do {
                if let updated = try await syncEngine.fetchContentIfNeeded(
                    documentId: cached.id,
                    modelContext: modelContext
                ) {
                    cachedDoc = updated
                }
            } catch {
                errorMessage = error.localizedDescription
            }

        case .byId(let id):
            do {
                document = try await service.fetchDocumentById(id: id)
            } catch {
                errorMessage = error.localizedDescription
            }

        case .byPath(let vaultId, let path):
            do {
                document = try await service.fetchDocument(vaultId: vaultId, path: path)
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }
}
