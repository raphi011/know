import Foundation

enum Loadable<T> {
	case idle
	case loading(prior: T?)
	case loaded(T)
	case failed(Error, prior: T?)

	var value: T? {
		switch self {
		case .loaded(let v): return v
		case .loading(let prior), .failed(_, let prior): return prior
		case .idle: return nil
		}
	}

	var isLoading: Bool {
		if case .loading = self { return true }
		return false
	}

	var error: Error? {
		if case .failed(let e, _) = self { return e }
		return nil
	}
}
