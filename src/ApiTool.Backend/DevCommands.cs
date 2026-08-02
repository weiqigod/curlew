using ApiTool.Backend.Notifications.Email;

namespace ApiTool.Backend;

/// <summary>
/// Dev-tools CLI dispatcher. Intercepts <c>dev *</c> args before the web host starts.
/// Returns a non-null exit code to suppress web host startup;
/// returns <c>null</c> to fall through to normal web host startup.
/// </summary>
internal static class DevCommands
{
    /// <summary>
    /// Dispatches dev subcommands. Call at the very top of Program.cs before <c>WebApplication.CreateBuilder</c>.
    /// </summary>
    /// <param name="args">Command-line arguments.</param>
    /// <returns>Exit code if a dev command was handled; <c>null</c> to fall through to web host.</returns>
    public static int? Run(string[] args)
    {
        if (args.Length == 0 || args[0] != "dev") return null;
        if (args.Length < 2) { PrintUsage(); return 2; }

        return args[1] switch
        {
            "email-preview" => RunEmailPreview(args.Skip(2).ToArray()),
            "upload-templates" => RunUploadTemplatesAsync(args.Skip(2).ToArray()).GetAwaiter().GetResult(),
            _ => UsageError($"Unknown dev subcommand: {args[1]}"),
        };
    }

    private static int RunEmailPreview(string[] rest)
    {
        if (rest.Length < 1)
        {
            Console.Error.WriteLine("usage: dev email-preview <slug>");
            return 2;
        }

        var slug = rest[0];
        var root = ResolveTemplatesRoot();
        if (root is null)
        {
            Console.Error.WriteLine("error: templates/email directory not found. Run from the repo root.");
            return 1;
        }

        try
        {
            var renderer = new EmailPreviewRenderer(new EmailTemplateLoader(root), new MjmlNetCompiler());
            Console.Out.Write(renderer.Render(slug));
            return 0;
        }
        catch (FileNotFoundException ex)
        {
            Console.Error.WriteLine($"error: {ex.Message}");
            return 1;
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine($"error rendering template '{slug}': {ex.Message}");
            return 1;
        }
    }

    private static async Task<int> RunUploadTemplatesAsync(string[] rest)
    {
        // Parse --api-key <value>
        string? apiKey = null;
        for (var i = 0; i < rest.Length - 1; i++)
        {
            if (rest[i] == "--api-key")
            {
                apiKey = rest[i + 1];
                break;
            }
        }

        if (string.IsNullOrEmpty(apiKey))
        {
            Console.Error.WriteLine("usage: dev upload-templates --api-key <SG.xxx>");
            return 2;
        }

        var root = ResolveTemplatesRoot();
        if (root is null)
        {
            Console.Error.WriteLine("error: templates/email directory not found. Run from the repo root.");
            return 1;
        }

        // Upload all slugs from the canonical M14 inventory (single source of truth).
        var slugs = EmailTemplateInventory.Slugs;
        if (slugs.Count == 0)
        {
            Console.Error.WriteLine("No template slugs defined in EmailTemplateInventory.");
            return 1;
        }

        var apiBase = Environment.GetEnvironmentVariable("SENDGRID_API_BASE") ?? "https://api.sendgrid.com";
        using var http = new HttpClient { BaseAddress = new Uri(apiBase) };
        var uploader = new SendGridTemplateUploader(http, apiKey);
        var loader = new EmailTemplateLoader(root);
        var compiler = new MjmlNetCompiler();
        var renderer = new EmailPreviewRenderer(loader, compiler);

        foreach (var slug in slugs)
        {
            try
            {
                var manifest = loader.LoadManifest(slug);
                var html = renderer.Render(slug);
                var id = await uploader.UploadAsync(slug, manifest.Subject, html, CancellationToken.None)
                    .ConfigureAwait(false);
                // Emit KEY=VALUE for GitHub Actions >> $GITHUB_OUTPUT consumption.
                Console.WriteLine($"APITOOL__SENDGRID__TEMPLATES__{slug.ToUpperInvariant()}={id}");
            }
            catch (Exception ex)
            {
                Console.Error.WriteLine($"error uploading '{slug}': {ex.Message}");
                return 1;
            }
        }

        return 0;
    }

    private static string? ResolveTemplatesRoot()
    {
        // 1. Try the current working directory (dotnet run from repo root).
        var candidate = Path.Combine(Directory.GetCurrentDirectory(), "templates", "email");
        if (Directory.Exists(candidate)) return candidate;

        // 2. Walk up from AppContext.BaseDirectory — handles dotnet run from any CWD.
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir is not null)
        {
            candidate = Path.Combine(dir.FullName, "templates", "email");
            if (Directory.Exists(candidate)) return candidate;
            dir = dir.Parent;
        }

        // 3. Walk up from the assembly's location (same as 2 for most .NET scenarios).
        dir = new DirectoryInfo(
            Path.GetDirectoryName(typeof(DevCommands).Assembly.Location) ?? AppContext.BaseDirectory);
        while (dir is not null)
        {
            candidate = Path.Combine(dir.FullName, "templates", "email");
            if (Directory.Exists(candidate)) return candidate;
            dir = dir.Parent;
        }

        return null;
    }

    private static int UsageError(string message)
    {
        Console.Error.WriteLine($"error: {message}");
        PrintUsage();
        return 2;
    }

    private static void PrintUsage()
    {
        Console.Error.WriteLine("usage:");
        Console.Error.WriteLine("  dev email-preview <slug>               Render template to stdout (no SendGrid required)");
        Console.Error.WriteLine("  dev upload-templates --api-key <key>   Upload compiled HTML to SendGrid Dynamic Templates");
    }
}
