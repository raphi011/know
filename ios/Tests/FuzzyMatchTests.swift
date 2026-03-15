import Testing
@testable import Know

@Test func matchesExactSubstring() {
    let result = fuzzyMatch(query: "config", target: "/app/config.md")
    #expect(result != nil)
    #expect(result!.matchedIndices.count == 6)
}

@Test func matchesFuzzyCharacters() {
    let result = fuzzyMatch(query: "acd", target: "abcdef")
    #expect(result != nil)
    #expect(result!.matchedIndices == [0, 2, 3])
}

@Test func returnsNilForNoMatch() {
    let result = fuzzyMatch(query: "xyz", target: "abcdef")
    #expect(result == nil)
}

@Test func emptyQueryReturnsNil() {
    let result = fuzzyMatch(query: "", target: "anything")
    #expect(result == nil)
}

@Test func caseInsensitive() {
    let result = fuzzyMatch(query: "ABC", target: "aXbXc")
    #expect(result != nil)
}

@Test func boundaryMatchScoresHigher() {
    guard let pathResult = fuzzyMatch(query: "conf", target: "/app/config.md"),
          let midResult = fuzzyMatch(query: "conf", target: "/appXconfigX.md") else {
        Issue.record("Expected non-nil results")
        return
    }
    #expect(pathResult.score > midResult.score)
}

@Test func consecutiveMatchScoresHigher() {
    guard let consecutive = fuzzyMatch(query: "abc", target: "abcdef"),
          let scattered = fuzzyMatch(query: "abc", target: "aXXbXXc") else {
        Issue.record("Expected non-nil results")
        return
    }
    #expect(consecutive.score > scattered.score)
}

@Test func queryLongerThanTargetReturnsNil() {
    let result = fuzzyMatch(query: "longquery", target: "short")
    #expect(result == nil)
}

@Test func matchesPathSegments() {
    let result = fuzzyMatch(query: "nai", target: "/Notes/AI/doc.md")
    #expect(result != nil)
}

@Test func prefersShorterPaths() {
    guard let short = fuzzyMatch(query: "doc", target: "/doc.md"),
          let long = fuzzyMatch(query: "doc", target: "/very/deeply/nested/path/to/doc.md") else {
        Issue.record("Expected non-nil results")
        return
    }
    #expect(short.score > long.score)
}

@Test func camelCaseBoundaryScoresHigher() {
    guard let camelResult = fuzzyMatch(query: "gv", target: "getVault"),
          let midResult = fuzzyMatch(query: "gv", target: "groovy") else {
        Issue.record("Expected non-nil results")
        return
    }
    #expect(camelResult.score > midResult.score)
}

@Test func separatorBoundaryBonus() {
    // All separators should give a boundary bonus
    for separator in ["/", "-", "_", " ", "."] {
        let target = "x\(separator)abc"
        guard let result = fuzzyMatch(query: "a", target: target) else {
            Issue.record("Expected match for separator '\(separator)'")
            continue
        }
        // Match after separator should score higher than mid-word
        guard let midResult = fuzzyMatch(query: "a", target: "xzabc") else {
            Issue.record("Expected match for mid-word")
            continue
        }
        #expect(result.score > midResult.score, "Separator '\(separator)' should give bonus")
    }
}

@Test func singleCharacterQuery() {
    let result = fuzzyMatch(query: "a", target: "abc")
    #expect(result != nil)
    #expect(result!.matchedIndices == [0])
}

@Test func exactLengthMatch() {
    let result = fuzzyMatch(query: "abc", target: "abc")
    #expect(result != nil)
    #expect(result!.matchedIndices == [0, 1, 2])
}
