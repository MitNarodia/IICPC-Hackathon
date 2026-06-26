import React, { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/atom-one-dark.css'; // Add a nice dark theme for code blocks
import { Card } from '@/components/ui/Card';
import { Search, Book } from 'lucide-react';

const docsContent = `
# IICPC Platform Documentation

Welcome to the distributed systems benchmarking platform.

## Getting Started

To submit your high-frequency trading bot to the platform, you must wrap it in a Docker container that exposes a WebSocket server on port \`8081\`.

### Example C++ Bot

\`\`\`cpp
#include <iostream>
#include <websocketpp/config/asio_no_tls.hpp>
#include <websocketpp/server.hpp>

typedef websocketpp::server<websocketpp::config::asio> server;

int main() {
    server echo_server;
    echo_server.init_asio();
    
    echo_server.set_message_handler([](websocketpp::connection_hdl hdl, server::message_ptr msg) {
        std::cout << "Received: " << msg->get_payload() << std::endl;
    });

    echo_server.listen(8081);
    echo_server.start_accept();
    echo_server.run();
}
\`\`\`

## Architecture Guidelines

- **Track 1:** Ensure your Dockerfile is optimized. Build times must be < 5 minutes.
- **Track 2:** Expect up to 10,000 concurrent WebSocket connections.
- **Track 3:** Your telemetry must not exceed 50MB/s of bandwidth.
`;

const navItems = [
  { section: 'Getting Started', items: ['Quickstart', 'Installation', 'Architecture'] },
  { section: 'API Reference', items: ['Submission API', 'Telemetry Webhooks', 'Bot Fleet Protocol'] },
  { section: 'Guides', items: ['Optimizing C++', 'Handling Backpressure', 'Writing Custom Drivers'] },
];

export function Docs() {
  const [activeItem, setActiveItem] = useState('Quickstart');

  return (
    <div className="flex h-full pb-12 gap-6">
      {/* Sidebar Navigation */}
      <Card className="w-64 h-full flex flex-col shrink-0 bg-card/50 border-gray-800">
        <div className="p-4 border-b border-gray-800">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-500" />
            <input
              type="text"
              placeholder="Search docs..."
              className="w-full pl-9 pr-4 py-2 bg-gray-900 border border-gray-800 rounded-md text-sm text-gray-200 focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
        </div>
        <div className="flex-1 overflow-y-auto p-4 space-y-6">
          {navItems.map((nav, i) => (
            <div key={i}>
              <h4 className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-3">{nav.section}</h4>
              <ul className="space-y-1">
                {nav.items.map(item => (
                  <li key={item}>
                    <button
                      onClick={() => setActiveItem(item)}
                      className={`w-full text-left px-3 py-1.5 rounded-md text-sm transition-colors ${activeItem === item ? 'bg-primary/10 text-primary font-medium' : 'text-gray-300 hover:bg-gray-800 hover:text-gray-100'}`}
                    >
                      {item}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </Card>

      {/* Main Content Area */}
      <Card className="flex-1 h-full overflow-y-auto">
        <div className="max-w-4xl mx-auto p-8 lg:p-12 prose prose-invert prose-blue max-w-none">
          {/* We use prose from tailwind typography, but we need to install it. I'll just use basic CSS for now if typography is missing, or react-markdown renders standard HTML tags which we can style. */}
          <div className="markdown-body">
            <div className="flex items-center gap-3 text-primary mb-6">
              <Book className="h-8 w-8" />
              <span className="text-xl font-bold">{activeItem} Guide</span>
            </div>
            
            <ReactMarkdown 
              rehypePlugins={[rehypeHighlight]}
              components={{
                h1: ({node, ...props}) => <h1 className="text-3xl font-bold text-gray-100 mt-8 mb-4 pb-2 border-b border-gray-800" {...props} />,
                h2: ({node, ...props}) => <h2 className="text-2xl font-semibold text-gray-100 mt-8 mb-4" {...props} />,
                h3: ({node, ...props}) => <h3 className="text-xl font-semibold text-gray-200 mt-6 mb-3" {...props} />,
                p: ({node, ...props}) => <p className="text-gray-300 leading-relaxed mb-4" {...props} />,
                ul: ({node, ...props}) => <ul className="list-disc pl-6 text-gray-300 space-y-2 mb-4" {...props} />,
                li: ({node, ...props}) => <li {...props} />,
                code: ({node, inline, className, children, ...props}: any) => {
                  const match = /language-(\w+)/.exec(className || '')
                  return !inline ? (
                    <code className={`${className} block rounded-md p-4 text-sm overflow-x-auto mb-6 bg-[#282c34]`} {...props}>
                      {children}
                    </code>
                  ) : (
                    <code className="bg-gray-800 text-gray-200 px-1.5 py-0.5 rounded text-sm font-mono" {...props}>
                      {children}
                    </code>
                  )
                }
              }}
            >
              {docsContent}
            </ReactMarkdown>
          </div>
        </div>
      </Card>
    </div>
  );
}
