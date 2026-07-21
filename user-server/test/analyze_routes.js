const fs = require('fs');
const path = require('path');

const frontendApiDir = '/Users/xiaofang/Documents/www/php/tggate/merchant/src/api';
const backendRouterFile = '/Users/xiaofang/Documents/www/php/tggate/marketing/internal/router/router.go';

// 1. Parse Frontend APIs
function getFrontendRoutes() {
    const files = fs.readdirSync(frontendApiDir).filter(f => f.endsWith('.js'));
    const routes = [];

    files.forEach(file => {
        const content = fs.readFileSync(path.join(frontendApiDir, file), 'utf-8');
        const regex = /http\.(get|post|put|delete)\(['"`](.*?)['"`]/g;
        let match;
        while ((match = regex.exec(content)) !== null) {
            let url = match[2];
            // Handle template literals loosely (replace ${...} with *)
            url = url.replace(/\$\{.*?\}/g, ':id'); 
            routes.push({
                source: file,
                method: match[1].toUpperCase(),
                url: url
            });
        }
    });
    return routes;
}

// 2. Parse Backend Routes
function getBackendRoutes() {
    const content = fs.readFileSync(backendRouterFile, 'utf-8');
    const routes = [];
    const lines = content.split('\n');
    
    let currentGroupPrefix = '';
    const groupStack = []; // track nested groups if any

    lines.forEach(line => {
        // Detect Group definition
        // public := r.Group("/api")
        // auth := r.Group("/api")
        // platform := r.Group("/api/platform")
        const groupMatch = line.match(/\w+\s*:=\s*r\.Group\("([^"]+)"\)/);
        if (groupMatch) {
            // This is a simplification. Real parsing is harder, but this catches the main groups in the provided router.go
            // The router.go provided has flat groups mainly.
            // But wait, the router.go uses variables like 'public', 'auth', 'platform'.
            // We need to map variable names to prefixes.
        }
    });

    // Better approach for this specific file structure:
    // The router.go has:
    // public := r.Group("/api")
    // auth := r.Group("/api")
    // platform := r.Group("/api/platform")
    
    // We can just look for .GET, .POST with the variable name to know the prefix.
    const prefixes = {
        'public': '/api',
        'auth': '/api',
        'platform': '/api/platform',
        'r': '' // root
    };

    const methodRegex = /(public|auth|platform|r)\.(GET|POST|PUT|DELETE)\("([^"]+)"/;
    
    lines.forEach(line => {
        const match = line.match(methodRegex);
        if (match) {
            const variable = match[1];
            const method = match[2];
            const pathSuffix = match[3];
            
            let fullPath = (prefixes[variable] || '') + pathSuffix;
            // Clean up double slashes if any
            fullPath = fullPath.replace('//', '/');
            
            routes.push({
                method: method,
                url: fullPath
            });
        }
    });

    return routes;
}

// 3. Compare
function analyze() {
    const frontend = getFrontendRoutes();
    const backend = getBackendRoutes();

    console.log(`Found ${frontend.length} frontend API calls.`);
    console.log(`Found ${backend.length} backend routes.`);

    const missing = [];

    frontend.forEach(fRoute => {
        // Normalize for comparison
        // Backend uses :id, Frontend might use :id or just append strings
        // We tried to convert ${} to :id
        
        // Exact match check
        const exact = backend.find(b => b.method === fRoute.method && b.url === fRoute.url);
        if (!exact) {
            // Try fuzzy match for parameters
            // e.g. /api/users/:id vs /api/users/${id} -> /api/users/:id
            
            // Convert backend :param to generic placeholder
            const backendRegex = backend.map(b => ({
                original: b,
                regex: new RegExp('^' + b.url.replace(/:[a-zA-Z0-9_]+/g, '[^/]+') + '$')
            }));

            const match = backendRegex.find(b => b.regex.test(fRoute.url.replace(/:id/g, '123'))); // try to simulate an ID
            
            if (!match) {
                // Double check if it's a known issue or false positive
                // e.g. frontend: /api/auto-reply/accounts/${id} -> /api/auto-reply/accounts/:id
                
                // Let's try to match by converting frontend :id back to :param pattern
                const fUrlPattern = fRoute.url.replace(/:id/g, ':[^/]+');
                
                // Simple check: does backend have this path with *some* param?
                const potential = backend.find(b => {
                     if (b.method !== fRoute.method) return false;
                     // Convert both to generic structure
                     const bStruct = b.url.replace(/:[a-zA-Z0-9_]+/g, '{}');
                     const fStruct = fRoute.url.replace(/:id/g, '{}');
                     return bStruct === fStruct;
                });

                if (!potential) {
                    missing.push(fRoute);
                }
            }
        }
    });

    if (missing.length > 0) {
        console.log('\n❌ Potential Missing Backend Routes for Frontend Calls:');
        missing.forEach(m => {
            console.log(`[${m.method}] ${m.url} (in ${m.source})`);
        });
    } else {
        console.log('\n✅ All frontend API calls seem to have matching backend routes.');
    }
}

analyze();
