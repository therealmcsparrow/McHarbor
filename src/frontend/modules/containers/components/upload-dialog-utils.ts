// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type UploadSelection = {
  files: Array<{ file: File; path: string }>;
  directories: string[];
};

type BrowserFileSystemEntryBase = {
  name: string;
};

type BrowserFileSystemFileEntry = BrowserFileSystemEntryBase & {
  isFile: true;
  isDirectory: false;
  file: (success: (file: File) => void, error?: (error: DOMException) => void) => void;
};

type BrowserFileSystemDirectoryReader = {
  readEntries: (
    success: (entries: BrowserFileSystemEntry[]) => void,
    error?: (error: DOMException) => void,
  ) => void;
};

type BrowserFileSystemDirectoryEntry = BrowserFileSystemEntryBase & {
  isFile: false;
  isDirectory: true;
  createReader: () => BrowserFileSystemDirectoryReader;
};

type BrowserFileSystemEntry = BrowserFileSystemFileEntry | BrowserFileSystemDirectoryEntry;

type DataTransferItemWithEntry = DataTransferItem & {
  webkitGetAsEntry?: () => BrowserFileSystemEntry | null;
};

function fileFromEntry(entry: BrowserFileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => {
    entry.file(resolve, reject);
  });
}

function readDirectoryEntries(reader: BrowserFileSystemDirectoryReader): Promise<BrowserFileSystemEntry[]> {
  return new Promise((resolve, reject) => {
    const entries: BrowserFileSystemEntry[] = [];
    const readBatch = () => {
      reader.readEntries(
        (batch) => {
          if (batch.length === 0) {
            resolve(entries);
            return;
          }
          entries.push(...batch);
          readBatch();
        },
        reject,
      );
    };
    readBatch();
  });
}

async function collectEntry(entry: BrowserFileSystemEntry, parentPath: string, selection: UploadSelection) {
  const relativePath = parentPath ? `${parentPath}/${entry.name}` : entry.name;
  if (entry.isFile) {
    selection.files.push({ file: await fileFromEntry(entry), path: relativePath });
    return;
  }

  selection.directories.push(relativePath);
  const entries = await readDirectoryEntries(entry.createReader());
  for (const child of entries) {
    await collectEntry(child, relativePath, selection);
  }
}

function uniqueDirectories(directories: string[]): string[] {
  return [...new Set(directories.filter(Boolean))];
}

function parentDirectories(path: string): string[] {
  const parts = path.split('/').filter(Boolean);
  parts.pop();
  return parts.map((_, index) => parts.slice(0, index + 1).join('/'));
}

export function mergeUploadSelections(current: UploadSelection, next: UploadSelection): UploadSelection {
  const filesByPath = new Map(current.files.map((item) => [item.path, item]));
  next.files.forEach((item) => filesByPath.set(item.path, item));
  return {
    files: [...filesByPath.values()],
    directories: uniqueDirectories([...current.directories, ...next.directories]),
  };
}

export function selectionFromFileList(fileList: FileList | null): UploadSelection {
  const files = Array.from(fileList ?? []).map((file) => ({
    file,
    path: (file.webkitRelativePath || file.name).replaceAll('\\', '/'),
  }));
  return { files, directories: uniqueDirectories(files.flatMap((item) => parentDirectories(item.path))) };
}

export async function selectionFromDataTransfer(dataTransfer: DataTransfer): Promise<UploadSelection> {
  const selection: UploadSelection = { files: [], directories: [] };
  const items = Array.from(dataTransfer.items ?? []);
  if (items.length > 0) {
    for (const item of items) {
      const entry = ((item as DataTransferItemWithEntry).webkitGetAsEntry?.() ?? null) as BrowserFileSystemEntry | null;
      if (entry) {
        await collectEntry(entry, '', selection);
        continue;
      }
      const file = item.getAsFile();
      if (file) selection.files.push({ file, path: file.name });
    }
  } else {
    selection.files = Array.from(dataTransfer.files).map((file) => ({ file, path: file.name }));
  }
  return { files: selection.files, directories: uniqueDirectories(selection.directories) };
}

export function totalUploadSize(selection: UploadSelection): number {
  return selection.files.reduce((total, item) => total + item.file.size, 0);
}
