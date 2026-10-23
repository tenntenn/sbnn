// Package filelock takes an exclusive lock on an open file, held against
// other processes as well as against other opens inside this one.
//
// It started as the lock that keeps two servers from interleaving records in
// the review log, and became a package of its own when a second file - the
// map of which repository's server listens on which port - needed the same
// promise.
package filelock
