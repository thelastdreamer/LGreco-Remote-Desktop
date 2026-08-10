using System;
using System.Runtime.InteropServices;

namespace RemoteDesktop.Client.Native;

public static class SharedMemory
{
    private const uint PageReadWrite = 0x04;
    private const uint FileMapWrite = 0x0002;
    private const uint FileMapRead = 0x0004;

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr CreateFileMapping(
        IntPtr hFile, IntPtr lpAttributes, uint flProtect,
        uint dwMaximumSizeHigh, uint dwMaximumSizeLow, string lpName);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr OpenFileMapping(
        uint dwDesiredAccess, bool bInheritHandle, string lpName);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr MapViewOfFile(
        IntPtr hFileMappingObject, uint dwDesiredAccess,
        uint dwFileOffsetHigh, uint dwFileOffsetLow, UIntPtr dwNumberOfBytesToMap);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool UnmapViewOfFile(IntPtr lpBaseAddress);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool CloseHandle(IntPtr hObject);

    public static IntPtr CreateOrOpen(int width, int height)
    {
        int size = width * height * 4;
        IntPtr hMap;

        hMap = OpenFileMapping(FileMapRead, false, "rd-video-frame");
        if (hMap != IntPtr.Zero)
            return hMap;

        hMap = CreateFileMapping(
            new IntPtr(-1), IntPtr.Zero, PageReadWrite,
            0, (uint)size, "rd-video-frame");
        return hMap;
    }

    public static IntPtr MapView(IntPtr hMap, int width, int height)
    {
        int size = width * height * 4;
        return MapViewOfFile(hMap, FileMapRead, 0, 0, new UIntPtr((uint)size));
    }

    public static void UnmapView(IntPtr addr)
    {
        UnmapViewOfFile(addr);
    }

    public static void Close(IntPtr hMap)
    {
        CloseHandle(hMap);
    }
}
