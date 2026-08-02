namespace ApiTool.Backend.Bootstrap;

/// <summary>Thrown when EF Core migrations fail to apply.</summary>
public sealed class MigrationFailedException(string message, Exception inner)
    : Exception(message, inner);
