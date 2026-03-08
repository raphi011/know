import SwiftUI

struct SearchResultRow: View {
    let result: SearchResult

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(result.title)
                .font(.body)
                .fontWeight(.medium)
                .lineLimit(1)

            Text(result.path)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)

            if let bestChunk = result.matchedChunks.max(by: { $0.score < $1.score }) {
                if let heading = bestChunk.headingPath, !heading.isEmpty {
                    Text(heading)
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }

                Text(bestChunk.snippet)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(3)
            }

            HStack(spacing: 8) {
                if let docType = result.docType {
                    Text(docType)
                        .font(.caption2)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(.fill.tertiary)
                        .clipShape(Capsule())
                }

                if !result.labels.isEmpty {
                    Text(result.labels.joined(separator: ", "))
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 4)
    }
}
