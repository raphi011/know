import MarkdownUI
import SwiftUI

enum DocumentReference {
    case byPath(vaultId: String, path: String)
    case byId(String)
}

struct DocumentView: View {
    let service: KnowhowService
    let reference: DocumentReference

    @State private var document: Document?
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
                }
            } else if let document {
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        if document.docType != nil || !document.labels.isEmpty {
                            HStack(spacing: 6) {
                                if let docType = document.docType {
                                    Text(docType)
                                        .font(.caption)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 3)
                                        .background(.fill.tertiary)
                                        .clipShape(Capsule())
                                }

                                ForEach(document.labels, id: \.self) { label in
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

                        Markdown(document.contentBody)
                            .textSelection(.enabled)
                    }
                    .padding()
                }
            }
        }
        .navigationTitle(document?.title ?? "Document")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await loadDocument()
        }
    }

    private func loadDocument() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            switch reference {
            case .byId(let id):
                document = try await service.fetchDocumentById(id: id)
            case .byPath(let vaultId, let path):
                document = try await service.fetchDocument(vaultId: vaultId, path: path)
            }
        } catch {
            errorMessage = error.localizedDescription
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
