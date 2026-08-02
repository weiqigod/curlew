namespace ApiTool.Backend.Notifications.Email;

/// <summary>Seam for compiling MJML markup to responsive HTML.</summary>
public interface IMjmlCompiler
{
    /// <summary>Compiles MJML markup to responsive HTML.</summary>
    /// <param name="mjml">The MJML source markup.</param>
    /// <returns>The compiled HTML string.</returns>
    /// <exception cref="InvalidOperationException">Thrown when the MJML contains compilation errors.</exception>
    string Compile(string mjml);
}
