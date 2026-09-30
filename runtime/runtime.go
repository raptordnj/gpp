// Package runtime is reserved for G++ runtime support.
//
// G++ v0.1 deliberately needs no runtime: classes, inheritance, interfaces,
// enums, static members and properties are all lowered to plain Go
// declarations, so generated programs depend only on the Go standard library
// and whatever packages they import themselves. Future features that cannot
// be expressed as a direct lowering (for example virtual dispatch across
// embedded classes) would live here.
package runtime

// Version is the G++ runtime version, matching the compiler version.
const Version = "0.1.0"
