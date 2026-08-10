using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace RemoteDesktop.Client.ViewModels;

public class SettingsViewModel : INotifyPropertyChanged
{
    private string _defaultServerUrl = "http://localhost:8080";
    private string _defaultResolution = "1280x720";
    private bool _audioEnabled = true;
    private bool _clipboardSync = true;
    private int _qualityLevel = 75;
    private int _framerate = 30;

    public string DefaultServerUrl
    {
        get => _defaultServerUrl;
        set { _defaultServerUrl = value; OnPropertyChanged(); }
    }

    public string DefaultResolution
    {
        get => _defaultResolution;
        set { _defaultResolution = value; OnPropertyChanged(); }
    }

    public bool AudioEnabled
    {
        get => _audioEnabled;
        set { _audioEnabled = value; OnPropertyChanged(); }
    }

    public bool ClipboardSync
    {
        get => _clipboardSync;
        set { _clipboardSync = value; OnPropertyChanged(); }
    }

    public int QualityLevel
    {
        get => _qualityLevel;
        set { _qualityLevel = value; OnPropertyChanged(); }
    }

    public int Framerate
    {
        get => _framerate;
        set { _framerate = value; OnPropertyChanged(); }
    }

    public event PropertyChangedEventHandler? PropertyChanged;
    protected void OnPropertyChanged([CallerMemberName] string? name = null)
    {
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
    }
}
