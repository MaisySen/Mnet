#!/usr/bin/env python3
"""Install or remove a private, opt-in Linux engineering prefix. No service/enrollment."""
import hashlib
import errno
import io
import json
import os
import pathlib
import platform
import stat
import subprocess
import sys
import tempfile
import tarfile

REPO = pathlib.Path(__file__).resolve().parent.parent
FILES = ('bin/mnet', 'bin/mnetd', 'share/applications/mnet-engineering.desktop', 'README.txt')
DIRS = ('bin', 'share', 'share/applications')
MANIFEST = 'INSTALLATION.json'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def open_dir(path, parent=None):
    return os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)


def private_owner(fd):
    info = os.fstat(fd)
    if info.st_uid != os.geteuid() or info.st_mode & 0o022:
        raise ValueError('directory must be owned by this user and not writable by others')


def read_file(root, name):
    fd = os.open(name, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=root)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid():
            raise ValueError('installed file is not a regular file owned by this user')
        with os.fdopen(os.dup(fd), 'rb') as stream:
            return stream.read(64 * 1024 * 1024), stat.S_IMODE(info.st_mode)
    finally:
        os.close(fd)


def write_file(root, name, data, mode):
    fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, mode, dir_fd=root)
    with os.fdopen(fd, 'wb') as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def exec_quote(value):
    # Desktop Entry Exec is not a shell. Escape its quoted argument grammar and
    # field-code percent signs, including paths containing spaces or dollars.
    argument = '"' + ''.join('\\' + c if c in '\\"`$' else '%%' if c == '%' else c for c in value) + '"'
    return argument.replace('\\', '\\\\')


def install(parent, leaf, absolute):
    status = subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=normal'], cwd=REPO, text=True)
    if status:
        raise ValueError('source tree is not clean; commit/review source before installing')
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=REPO, text=True).strip()
    archive = subprocess.check_output(['git', 'archive', '--format=tar', revision], cwd=REPO)
    os.mkdir(leaf, 0o700, dir_fd=parent)
    root = open_dir(leaf, parent)
    try:
        for name in DIRS:
            os.mkdir(name, 0o700, dir_fd=root)
        # Building happens only after exclusive prefix creation. A failed build
        # leaves a partial private prefix; it is not silently removed or runnable
        # through an installed desktop entry.
        env = os.environ.copy()
        env.pop('GOOS', None)
        env.pop('GOARCH', None)
        env['GOFLAGS'] = ''
        env['GOWORK'] = 'off'
        with tempfile.TemporaryDirectory(prefix='mnet-install-build.') as temporary:
            source = pathlib.Path(temporary) / 'source'
            source.mkdir(mode=0o700)
            with tarfile.open(fileobj=io.BytesIO(archive)) as snapshot:
                snapshot.extractall(source, filter='data')
            for name in ('mnet', 'mnetd'):
                target = pathlib.Path(temporary) / name
                subprocess.run(['go', 'build', '-mod=readonly', '-buildvcs=false', '-o', str(target), './cmd/' + name], cwd=source, env=env, check=True)
                write_file(root, 'bin/' + name, target.read_bytes(), 0o700)
        subprocess.run(['/proc/self/fd/' + str(root) + '/bin/mnet', 'version'], pass_fds=(root,), check=True)
        note = ('Mnet local Linux engineering installation\n\n'
                'Run bin/mnet open on a supported local X11 desktop. This is not a consumer release.\n'
                'This prefix contains the CLI and daemon executables plus a desktop-entry file.\n'
                'Nothing was added to your desktop menus, PATH, startup, systemd or global directories.\n'
                'No daemon was started. No network, enrollment, identity, key or account was created.\n'
                'The home opens without daemon access. Network and Remote Desktop tools still need\n'
                'separately configured local daemon state. Multies first-run setup is unavailable.\n'
                'Shared Desk pairing is not enabled by installing these files.\n'
                'This build requires Linux X11; Wayland-only and other platforms are unsupported here.\n'
                'Remove only with the matching source installer uninstall command after review.\n'
                'Changed or extra files cause removal to stop before deleting anything.\n').encode()
        note += b'\nThird-party notices\n\n' + (REPO / 'THIRD_PARTY_NOTICES.txt').read_bytes()
        write_file(root, 'README.txt', note, 0o600)
        desktop = ('[Desktop Entry]\nVersion=1.0\nType=Application\nName=Mnet (engineering)\n'
                   'Comment=Local engineering tools; Multies setup is not connected\n'
                   'Exec=' + exec_quote(str(absolute / 'bin/mnet')) + ' open\n'
                   'Terminal=false\nCategories=Network;\nStartupNotify=false\n').encode()
        write_file(root, FILES[2], desktop, 0o600)
        entries = {}
        for name in FILES:
            data, mode = read_file(root, name)
            entries[name] = {'sha256': digest(data), 'mode': mode}
        write_file(root, MANIFEST, (json.dumps({'version': 1, 'engineering_only': True, 'source_commit': revision, 'source_archive_sha256': digest(archive), 'files': entries}, indent=2) + '\n').encode(), 0o600)
        os.fsync(root)
        os.fsync(parent)
    except Exception:
        print('Installation incomplete. Private partial prefix retained; do not use it.', file=sys.stderr)
        raise
    finally:
        os.close(root)
    print('Installed private engineering prefix. No service, account, enrollment or desktop-menu change.')


def uninstall(parent, leaf):
    root = open_dir(leaf, parent)
    descriptors = []
    try:
        private_owner(root)
        raw, mode = read_file(root, MANIFEST)
        manifest = json.loads(raw)
        if mode != 0o600 or set(manifest) != {'version', 'engineering_only', 'source_commit', 'source_archive_sha256', 'files'} or manifest['version'] != 1 or manifest['engineering_only'] is not True or set(manifest['files']) != set(FILES) or not isinstance(manifest['source_commit'], str) or len(manifest['source_commit']) != 40 or any(c not in '0123456789abcdef' for c in manifest['source_commit']) or not isinstance(manifest['source_archive_sha256'], str) or len(manifest['source_archive_sha256']) != 64 or any(c not in '0123456789abcdef' for c in manifest['source_archive_sha256']):
            raise ValueError('installation manifest does not match this installer')
        expected = {'': {'bin', 'share', 'README.txt', MANIFEST}, 'bin': {'mnet', 'mnetd'}, 'share': {'applications'}, 'share/applications': {'mnet-engineering.desktop'}}
        for name, names in expected.items():
            fd = root if not name else open_dir(name, root)
            if fd != root:
                descriptors.append(fd)
            private_owner(fd)
            if set(os.listdir(fd)) != names:
                raise ValueError('extra or missing files; nothing removed')
        for name in FILES:
            data, current_mode = read_file(root, name)
            if manifest['files'][name] != {'sha256': digest(data), 'mode': current_mode}:
                raise ValueError('installed file changed; nothing removed')
        # Linux rejects writable opens of an executing image (ETXTBSY), and
        # while the writer stays open a new exec of that inode is refused.
        # Check the two actual installed inodes, not every process on the host.
        # Never truncate/write the files; hold both descriptors through removal.
        for name in ('bin/mnet', 'bin/mnetd'):
            expected = os.stat(name, dir_fd=root, follow_symlinks=False)
            try:
                fd = os.open(name, os.O_WRONLY | os.O_NONBLOCK | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=root)
            except OSError as error:
                if error.errno == errno.ETXTBSY:
                    raise ValueError('installed executable is still running; close it first; nothing removed') from None
                raise ValueError('installed executable removal guard unavailable; nothing removed') from None
            descriptors.append(fd)
            opened = os.fstat(fd)
            if not stat.S_ISREG(opened.st_mode) or (expected.st_dev, expected.st_ino) != (opened.st_dev, opened.st_ino):
                raise ValueError('installed executable changed while acquiring removal guard; nothing removed')
        # All checks complete before deleting anything. Removal is limited to
        # the fixed manifest paths; never recursively remove a supplied path.
        for name in FILES + (MANIFEST,):
            os.unlink(name, dir_fd=root)
        for name in reversed(DIRS):
            os.rmdir(name, dir_fd=root)
        os.fsync(root)
        current = os.stat(leaf, dir_fd=parent, follow_symlinks=False)
        original = os.fstat(root)
        if (current.st_dev, current.st_ino) != (original.st_dev, original.st_ino):
            raise ValueError('prefix moved during removal; changed parent entry retained')
        os.rmdir(leaf, dir_fd=parent)
        os.fsync(parent)
    finally:
        for fd in descriptors:
            os.close(fd)
        os.close(root)
    print('Removed unchanged engineering prefix. Daemon state and all other paths were untouched.')


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in ('install', 'uninstall'):
        raise ValueError('usage: python3 scripts/linux-local-install.py install|uninstall ABSOLUTE_PRIVATE_PREFIX')
    if platform.system() != 'Linux' or os.geteuid() == 0:
        raise ValueError('engineering installer requires non-root Linux; no privileged fallback')
    prefix = pathlib.Path(sys.argv[2])
    if not prefix.is_absolute() or prefix.name in ('', '.', '..') or any(c in str(prefix) for c in '\n\r\x00%'):
        raise ValueError('prefix must be absolute and contain no line breaks or percent field-code characters')
    parent = open_dir(str(prefix.parent))
    try:
        private_owner(parent)
        if sys.argv[1] == 'install':
            install(parent, prefix.name, prefix)
        else:
            uninstall(parent, prefix.name)
    finally:
        os.close(parent)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print('Refused: ' + str(error), file=sys.stderr)
        sys.exit(1)
