/**
 * What `node --test` needs before it can load the page's own modules.
 *
 * Two things stand in the way, and neither is about the code under test.
 * The bundler resolves extensionless imports and Node does not, so a resolve
 * hook adds the .ts back. And the Markdown sanitiser configures itself
 * against a real DOM the moment it is imported, which a test process has
 * none of; nothing here renders Markdown, so it is stood in for by an object
 * with the two methods that get called.
 *
 * The stand-in keeps the callbacks it is handed rather than dropping them.
 * One of the things markdown.ts configures on import is the hook that
 * decides which links open in a tab of their own, and a test that wants to
 * ask it about an href has no other way to reach it: the hook is registered
 * at import time and never exported.
 */
import { existsSync } from 'node:fs'
import { registerHooks } from 'node:module'
import { fileURLToPath } from 'node:url'

const domPurifyStub =
  'data:text/javascript,' +
  encodeURIComponent(
    'const hooks = {};' +
      'const stub = { isSupported: false, hooks, ' +
      'addHook(name, cb) { (hooks[name] ??= []).push(cb) }, ' +
      'removeHook(name) { delete hooks[name] }, ' +
      'sanitize(html) { throw new Error("this test process has no DOM to sanitise " + typeof html + " in") } };' +
      'export default stub',
  )

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier === 'dompurify') return { url: domPurifyStub, shortCircuit: true }
    if (specifier.startsWith('.') && !/\.[cm]?[jt]sx?$/.test(specifier)) {
      const url = new URL(`${specifier}.ts`, context.parentURL)
      if (existsSync(fileURLToPath(url))) return { url: url.href, shortCircuit: true }
    }
    return nextResolve(specifier, context)
  },
})
