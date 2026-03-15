import SwiftData
import SwiftUI

struct QuickPickerView: View {
    let vaultId: String
    let service: KnowService
    let onSelect: (String) -> Void
    let onDismiss: () -> Void

    @Environment(\.modelContext) private var modelContext
    @State private var viewModel = QuickPickerViewModel()
    @FocusState private var isSearchFocused: Bool
    @State private var createError: String?

    var body: some View {
        VStack(spacing: 0) {
            searchField
            Divider()
            resultsList
            #if os(macOS)
            keyboardHints
            #endif
        }
        #if os(macOS)
        .frame(width: 600, height: 400)
        #endif
        .onAppear {
            viewModel.load(vaultId: vaultId, modelContext: modelContext)
            isSearchFocused = true
        }
        .overlay {
            if let error = viewModel.loadError {
                ContentUnavailableView {
                    Label("Error", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(error)
                }
            }
        }
    }

    // MARK: - Search Field

    private var searchField: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Find or create a note...", text: $viewModel.query)
                .textFieldStyle(.plain)
                .focused($isSearchFocused)
                #if os(macOS)
                .onKeyPress(.upArrow) {
                    viewModel.moveSelection(by: -1)
                    return .handled
                }
                .onKeyPress(.downArrow) {
                    viewModel.moveSelection(by: 1)
                    return .handled
                }
                .onKeyPress(.return) {
                    if NSEvent.modifierFlags.contains(.shift) {
                        handleCreate()
                    } else {
                        handleSelect()
                    }
                    return .handled
                }
                .onKeyPress(.escape) {
                    onDismiss()
                    return .handled
                }
                #endif
            if !viewModel.query.isEmpty {
                Button {
                    viewModel.query = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
    }

    // MARK: - Results

    private var resultsList: some View {
        ScrollViewReader { proxy in
            List {
                ForEach(Array(viewModel.results.enumerated()), id: \.element.id) { index, item in
                    Button {
                        selectItem(item)
                    } label: {
                        QuickPickerRow(
                            item: item,
                            isSelected: index == viewModel.selectedIndex
                        )
                    }
                    .buttonStyle(.plain)
                    .id(item.id)
                    .listRowInsets(EdgeInsets(top: 2, leading: 8, bottom: 2, trailing: 8))
                }

                if viewModel.canCreate {
                    Button {
                        handleCreate()
                    } label: {
                        createRow
                    }
                    .buttonStyle(.plain)
                    .id("__create__")
                    .listRowInsets(EdgeInsets(top: 2, leading: 8, bottom: 2, trailing: 8))
                }

                if let createError {
                    Text(createError)
                        .font(.caption)
                        .foregroundStyle(.red)
                        .listRowInsets(EdgeInsets(top: 2, leading: 16, bottom: 2, trailing: 8))
                }
            }
            .listStyle(.plain)
            .onChange(of: viewModel.selectedIndex) { _, newIndex in
                if newIndex < viewModel.results.count {
                    proxy.scrollTo(viewModel.results[newIndex].id, anchor: .center)
                } else if viewModel.canCreate {
                    proxy.scrollTo("__create__", anchor: .center)
                }
            }
        }
    }

    private var createRow: some View {
        HStack(spacing: 8) {
            Image(systemName: "plus.circle")
                .foregroundStyle(Color.accentColor)
            Text("Create ")
                .foregroundStyle(.secondary)
            + Text(viewModel.createPath)
                .foregroundStyle(Color.accentColor)
                .bold()

            Spacer()

            #if os(macOS)
            Text("shift ↵")
                .font(.caption2)
                .foregroundStyle(.tertiary)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(.fill.quaternary)
                .clipShape(RoundedRectangle(cornerRadius: 4))
            #endif
        }
        .padding(.vertical, 6)
        .padding(.horizontal, 8)
        .background(viewModel.isCreateSelected ? Color.accentColor.opacity(0.15) : .clear)
        .clipShape(RoundedRectangle(cornerRadius: 6))
    }

    // MARK: - Keyboard Hints (macOS)

    #if os(macOS)
    private var keyboardHints: some View {
        HStack(spacing: 16) {
            hintLabel("↑↓", "navigate")
            hintLabel("↵", "open")
            hintLabel("shift ↵", "create")
            hintLabel("esc", "dismiss")
        }
        .font(.caption2)
        .foregroundStyle(.secondary)
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity)
        .background(.bar)
    }

    private func hintLabel(_ key: String, _ action: String) -> some View {
        HStack(spacing: 4) {
            Text(key)
                .padding(.horizontal, 4)
                .padding(.vertical, 1)
                .background(.fill.quaternary)
                .clipShape(RoundedRectangle(cornerRadius: 3))
            Text(action)
        }
    }
    #endif

    // MARK: - Actions

    private func selectItem(_ item: QuickPickerItem) {
        onSelect(item.path)
    }

    private func handleSelect() {
        if viewModel.isCreateSelected {
            handleCreate()
        } else if let item = viewModel.selectedItem() {
            selectItem(item)
        }
    }

    private func handleCreate() {
        guard viewModel.canCreate else { return }
        let path = viewModel.createPath
        createError = nil

        Task {
            do {
                let doc = try await service.createDocument(vaultId: vaultId, path: path)
                selectItem(QuickPickerItem(
                    path: doc.path,
                    title: doc.title,
                    labels: [],
                    docType: nil,
                    score: 0,
                    matchedIndices: [],
                    isRecent: false
                ))
            } catch is CancellationError {
                return
            } catch {
                createError = error.localizedDescription
            }
        }
    }
}
