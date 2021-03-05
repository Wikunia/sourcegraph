// @ts-check

/** @type {jest.InitialOptions} */
const config = require('../../jest.config.base')

/** @type {jest.InitialOptions} */
module.exports = {
  ...config,
  displayName: 'ui-kit-legacy-shared',
  rootDir: __dirname,
}
