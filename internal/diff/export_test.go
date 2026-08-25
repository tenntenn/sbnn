package diff

// SplitContainersForTest exposes splitContainers to the package's external
// test, which is where the rest of the reconstruction tests live.
func SplitContainersForTest(line string) (cont, rest string) { return splitContainers(line) }
