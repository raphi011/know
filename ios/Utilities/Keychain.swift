import Foundation
import Security

/// Thin wrapper around the Security framework for storing credentials.
enum Keychain {
    private static let service = "com.knowhow.ios"

    static func save(key: String, value: String) throws {
        let data = Data(value.utf8)

        let deleteQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        let deleteStatus = SecItemDelete(deleteQuery as CFDictionary)
        if deleteStatus != errSecSuccess && deleteStatus != errSecItemNotFound
            && deleteStatus != errSecMissingEntitlement {
            throw KeychainError.operationFailed(deleteStatus)
        }

        let addQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
        ]

        let status = SecItemAdd(addQuery as CFDictionary, nil)
        // errSecMissingEntitlement in simulator without code signing — save is best-effort
        guard status == errSecSuccess || status == errSecMissingEntitlement else {
            throw KeychainError.operationFailed(status)
        }
    }

    static func load(key: String) throws -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)

        switch status {
        case errSecSuccess:
            guard let data = result as? Data,
                  let string = String(data: data, encoding: .utf8) else {
                throw KeychainError.dataCorrupted
            }
            return string
        case errSecItemNotFound, errSecMissingEntitlement:
            // errSecMissingEntitlement (-34018) occurs in simulator builds without code signing
            return nil
        default:
            throw KeychainError.operationFailed(status)
        }
    }

    static func delete(key: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        let status = SecItemDelete(query as CFDictionary)
        if status != errSecSuccess && status != errSecItemNotFound
            && status != errSecMissingEntitlement {
            throw KeychainError.operationFailed(status)
        }
    }
}

enum KeychainError: LocalizedError {
    case operationFailed(OSStatus)
    case dataCorrupted

    var errorDescription: String? {
        switch self {
        case .operationFailed(let status):
            return "Keychain operation failed (status: \(status))"
        case .dataCorrupted:
            return "Keychain data could not be decoded"
        }
    }
}
