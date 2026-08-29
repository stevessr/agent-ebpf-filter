# IR

The IR models fixed-layout structs, maps, probe attachment descriptors, scalar expressions and a small statement set. It intentionally has no Promise, exception, dynamic object or prototype concepts, making C and future native-BPF backends share the same safety boundary.
