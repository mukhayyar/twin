import Foundation

/// Just enough Mach-O reading to tell whether two binaries hold the same code.
enum MachO {
    /// The LC_UUID of every architecture in a binary (thin or universal). The linker sets these;
    /// unlike the file's bytes, they don't change when the file is signed again, which every
    /// twin's copy of libtwinisolate is.
    static func uuids(_ data: Data) -> Set<UUID> {
        func u32(_ offset: Int, bigEndian: Bool = false) -> Int? {
            guard offset >= 0, offset + 4 <= data.count else { return nil }
            let raw = data.withUnsafeBytes { $0.loadUnaligned(fromByteOffset: offset, as: UInt32.self) }
            return Int(bigEndian ? UInt32(bigEndian: raw) : UInt32(littleEndian: raw))
        }
        func u64BE(_ offset: Int) -> Int? {
            guard offset >= 0, offset + 8 <= data.count else { return nil }
            return Int(UInt64(bigEndian: data.withUnsafeBytes { $0.loadUnaligned(fromByteOffset: offset, as: UInt64.self) }))
        }

        // Universal binaries start with a big-endian fat header listing each slice.
        var slices: [Int] = []
        switch u32(0, bigEndian: true) {
        case 0xCAFE_BABE: // fat_arch: cputype, cpusubtype, offset, size, align
            for i in 0..<(u32(4, bigEndian: true) ?? 0) {
                if let offset = u32(8 + i * 20 + 8, bigEndian: true) { slices.append(offset) }
            }
        case 0xCAFE_BABF: // fat_arch_64: cputype, cpusubtype, offset (8), size (8), align, reserved
            for i in 0..<(u32(4, bigEndian: true) ?? 0) {
                if let offset = u64BE(8 + i * 32 + 8) { slices.append(offset) }
            }
        default:
            slices = [0]
        }

        var found = Set<UUID>()
        for slice in slices {
            let header: Int
            switch u32(slice) {
            case 0xFEED_FACF: header = 32 // mach_header_64
            case 0xFEED_FACE: header = 28 // mach_header
            default: continue
            }
            guard let count = u32(slice + 16) else { continue }
            var command = slice + header
            for _ in 0..<count {
                guard let kind = u32(command), let size = u32(command + 4), size >= 8 else { break }
                if kind == 0x1B, command + 24 <= data.count { // LC_UUID
                    let uuid = data.withUnsafeBytes { $0.loadUnaligned(fromByteOffset: command + 8, as: uuid_t.self) }
                    found.insert(UUID(uuid: uuid))
                }
                command += size
            }
        }
        return found
    }
}
