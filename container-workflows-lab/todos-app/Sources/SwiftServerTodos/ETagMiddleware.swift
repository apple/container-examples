import CryptoKit
//import Crypto
import Vapor

struct ETagMiddleware: AsyncMiddleware {
    func respond(to request: Request, chainingTo next: any AsyncResponder) async throws -> Response {
        let response = try await next.respond(to: request)

        // OpenAPI handlers stream their bodies, so collect the bytes before hashing.
        guard let buffer = try await response.body.collect(on: request.eventLoop).get() else {
            return response
        }
        response.body = .init(buffer: buffer)

        let hash = SHA256.hash(data: Data(buffer.readableBytesView))
            .map { String(format: "%02x", $0) }
            .joined()
        response.headers.replaceOrAdd(name: .eTag, value: "\"\(hash)\"")
        return response
    }
}