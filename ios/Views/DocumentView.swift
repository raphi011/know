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
                    VStack(alignment: .leading, spacing: 16) {
                        VStack(alignment: .leading, spacing: 8) {
                            Text(displayTitle)
                                .font(.largeTitle)
                                .fontWeight(.bold)

                            HStack(spacing: 12) {
                                let docType = document?.docType ?? cachedDoc?.docType
                                if let docType {
                                    Text(docType)
                                        .font(.caption)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 3)
                                        .background(.fill.tertiary)
                                        .clipShape(Capsule())
                                }

                                let path = document?.path ?? cachedDoc?.path ?? ""
                                Text(path)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }

                            let labels = document?.labels ?? cachedDoc?.labels ?? []
                            if !labels.isEmpty {
                                FlowLayout(spacing: 6) {
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
                        }

                        Divider()

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

// MARK: - Flow Layout for Labels

/// Simple horizontal flow layout that wraps to next line.
private struct FlowLayout: Layout {
    let spacing: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let result = arrange(proposal: proposal, subviews: subviews)
        return result.size
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let result = arrange(proposal: proposal, subviews: subviews)
        for (index, position) in result.positions.enumerated() {
            subviews[index].place(
                at: CGPoint(x: bounds.minX + position.x, y: bounds.minY + position.y),
                proposal: .unspecified
            )
        }
    }

    private func arrange(proposal: ProposedViewSize, subviews: Subviews) -> (size: CGSize, positions: [CGPoint]) {
        let maxWidth = proposal.width ?? .infinity
        var positions: [CGPoint] = []
        var x: CGFloat = 0
        var y: CGFloat = 0
        var rowHeight: CGFloat = 0
        var totalHeight: CGFloat = 0

        for subview in subviews {
            let size = subview.sizeThatFits(.unspecified)

            if x + size.width > maxWidth, x > 0 {
                x = 0
                y += rowHeight + spacing
                rowHeight = 0
            }

            positions.append(CGPoint(x: x, y: y))
            rowHeight = max(rowHeight, size.height)
            x += size.width + spacing
            totalHeight = y + rowHeight
        }

        return (CGSize(width: maxWidth, height: totalHeight), positions)
    }
}
