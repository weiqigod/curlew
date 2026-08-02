using Xunit;

// Disable all test parallelism within this assembly to prevent SQLite in-memory
// concurrency conflicts between test collections.
[assembly: CollectionBehavior(DisableTestParallelization = true)]
