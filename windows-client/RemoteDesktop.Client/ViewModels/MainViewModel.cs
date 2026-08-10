using System.ComponentModel;
using System.Runtime.CompilerServices;

namespace RemoteDesktop.Client.ViewModels;

public class MainViewModel : INotifyPropertyChanged
{
    private string _serverUrl = "http://localhost:8080";
    private string _username = "";
    private string _status = "Disconnected";
    private string _resolution = "";
    private bool _isLoggedIn;
    private bool _isBusy;

    public string ServerUrl
    {
        get => _serverUrl;
        set { _serverUrl = value; OnPropertyChanged(); }
    }

    public string Username
    {
        get => _username;
        set { _username = value; OnPropertyChanged(); }
    }

    public string Status
    {
        get => _status;
        set { _status = value; OnPropertyChanged(); }
    }

    public string Resolution
    {
        get => _resolution;
        set { _resolution = value; OnPropertyChanged(); }
    }

    public bool IsLoggedIn
    {
        get => _isLoggedIn;
        set { _isLoggedIn = value; OnPropertyChanged(); OnPropertyChanged(nameof(IsNotLoggedIn)); }
    }

    public bool IsNotLoggedIn => !_isLoggedIn;

    public bool IsBusy
    {
        get => _isBusy;
        set { _isBusy = value; OnPropertyChanged(); }
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    protected void OnPropertyChanged([CallerMemberName] string? name = null)
    {
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
    }
}
