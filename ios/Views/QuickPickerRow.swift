import SwiftUI

struct QuickPickerRow: View {
    let item: QuickPickerItem
    let isSelected: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            highlightedPath
                .font(.body)
                .lineLimit(1)

            if item.title != filename {
                Text(item.title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            if !item.labels.isEmpty || item.docType != nil {
                HStack(spacing: 6) {
                    if let docType = item.docType {
                        Text(docType)
                            .font(.caption2)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .background(.fill.tertiary)
                            .clipShape(Capsule())
                    }
                    if !item.labels.isEmpty {
                        Text(item.labels.joined(separator: ", "))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
            }
        }
        .padding(.vertical, 4)
        .padding(.horizontal, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(isSelected ? Color.accentColor.opacity(0.15) : .clear)
        .clipShape(RoundedRectangle(cornerRadius: 6))
    }

    private var filename: String {
        let name = item.path.components(separatedBy: "/").last ?? item.path
        if name.hasSuffix(".md") {
            return String(name.dropLast(3))
        }
        return name
    }

    private var highlightedPath: Text {
        if item.isRecent || item.matchedIndices.isEmpty {
            return Text(item.path)
        }

        let chars = Array(item.path)
        let matchedSet = Set(item.matchedIndices)
        var result = Text("")

        for (index, char) in chars.enumerated() {
            let segment = Text(String(char))
            if matchedSet.contains(index) {
                result = result + segment.bold().foregroundColor(.accentColor)
            } else {
                result = result + segment
            }
        }

        return result
    }
}
