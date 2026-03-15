import MarkdownUI
import SwiftUI

struct DocumentView: View {
	let service: KnowService
	let vaultId: String
	let path: String?

	@State private var content: Loadable<Document> = .idle
	@State private var showSpinner = false

	var body: some View {
		Group {
			if let error = content.error, content.value == nil {
				ContentUnavailableView {
					Label("Error", systemImage: "exclamationmark.triangle")
				} description: {
					Text(error.localizedDescription)
				}
			} else if let doc = content.value {
				ScrollView {
					VStack(alignment: .leading, spacing: 12) {
						if doc.docType != nil || !doc.labels.isEmpty {
							HStack(spacing: 6) {
								if let docType = doc.docType {
									Text(docType)
										.font(.caption)
										.padding(.horizontal, 8)
										.padding(.vertical, 3)
										.background(.fill.tertiary)
										.clipShape(Capsule())
								}

								ForEach(doc.labels, id: \.self) { label in
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

						Markdown(doc.displayBody)
							.textSelection(.enabled)
					}
					.frame(maxWidth: 700, alignment: .leading)
					.frame(maxWidth: .infinity, alignment: .leading)
					.padding()
				}
				.overlay(alignment: .topTrailing) {
					if content.isLoading {
						ProgressView()
							.padding(8)
					}
				}
			} else if content.isLoading && showSpinner {
				ProgressView()
			} else {
				ContentUnavailableView("Select a Document", systemImage: "doc.text")
			}
		}
		.navigationTitle(content.value?.title ?? "")
		#if os(iOS)
		.navigationBarTitleDisplayMode(.inline)
		#endif
		.task(id: path) {
			guard let path else {
				content = .idle
				showSpinner = false
				return
			}
			await loadDocument(vaultId: vaultId, path: path)
		}
	}

	private func loadDocument(vaultId: String, path: String) async {
		content = .loading(prior: content.value)
		showSpinner = false

		let spinnerTask = Task {
			try? await Task.sleep(for: .milliseconds(300))
			if !Task.isCancelled { showSpinner = true }
		}
		defer {
			spinnerTask.cancel()
			showSpinner = false
		}

		do {
			let newDoc = try await service.fetchDocument(vaultId: vaultId, path: path)
			content = .loaded(newDoc)
		} catch is CancellationError {
			return
		} catch {
			content = .failed(error, prior: content.value)
		}
	}
}
