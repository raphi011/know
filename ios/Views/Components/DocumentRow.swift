import SwiftUI

struct DocumentRow: View {
    let document: CachedDocument

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(document.title)
                .font(.body)
                .lineLimit(1)

            HStack(spacing: 8) {
                if let docType = document.docType {
                    Text(docType)
                        .font(.caption)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(.fill.tertiary)
                        .clipShape(Capsule())
                }

                if !document.labels.isEmpty {
                    Text(document.labels.joined(separator: ", "))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 2)
    }
}
