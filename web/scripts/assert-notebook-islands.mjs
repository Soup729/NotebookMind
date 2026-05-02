import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = process.cwd();
const files = [
  {
    path: 'src/app/notebooks/[id]/page.tsx',
    forbidden: [
      "@/components/chat/ChatPanel",
      "@/components/export/ExportDialog",
      "@/components/export/ExportTaskTray",
      "@/components/pdf/PdfViewer",
      "@/components/workspace/NotebookWorkspace",
    ],
  },
  {
    path: 'src/components/workspace/NotebookWorkspace.tsx',
    forbidden: ["@/components/workspace/KnowledgeGraphPanel"],
  },
];

const failures = [];

for (const file of files) {
  const source = readFileSync(join(root, file.path), 'utf8');

  for (const modulePath of file.forbidden) {
    const staticImport = new RegExp(`import\\s+(?:type\\s+)?(?:[^'"]+\\s+from\\s+)?['"]${escapeRegExp(modulePath)}['"]`);
    if (staticImport.test(source)) {
      failures.push(`${file.path} statically imports ${modulePath}`);
    }
  }
}

const rootPage = readFileSync(join(root, 'src/app/page.tsx'), 'utf8');
const rootForbidden = [
  "'use client'",
  '"use client"',
  'useRouter',
  'router.replace',
];

for (const marker of rootForbidden) {
  if (rootPage.includes(marker)) {
    failures.push(`src/app/page.tsx contains ${marker}; root redirect must stay server-side`);
  }
}

if (!rootPage.includes("redirect('/notebooks')") && !rootPage.includes('redirect("/notebooks")')) {
  failures.push('src/app/page.tsx must redirect to /notebooks with next/navigation redirect()');
}

const chatPanel = readFileSync(join(root, 'src/components/chat/ChatPanel.tsx'), 'utf8');
const staticChatMessageImport = /import\s+\{\s*ChatMessage\s*\}\s+from\s+['"]\.\/ChatMessage['"]/;

if (staticChatMessageImport.test(chatPanel)) {
  failures.push('src/components/chat/ChatPanel.tsx statically imports ChatMessage; Markdown rendering must stay lazy');
}

if (!chatPanel.includes("import('./ChatMessage')") && !chatPanel.includes('import("./ChatMessage")')) {
  failures.push('src/components/chat/ChatPanel.tsx must dynamically import ChatMessage');
}

if (failures.length > 0) {
  console.error('Notebook island split check failed:');
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exit(1);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
