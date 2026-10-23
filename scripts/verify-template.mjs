import { parseProject } from '@icm/project-protocol'

const template = {
  schemaVersion: 74,
  id: 'PROJECT-ID',
  name: 'DRAWING-NAME',
  topDocumentId: 'DOCUMENT-ID',
  defaults: {
    instance: { mirror: 'none', rotation: 0 },
    instancesByType: {},
    label: { alignment: 'middle', locked: false, rotation: 0 },
    routeLeg: { mode: 'manual' }
  },
  documents: [
    {
      id: 'DOCUMENT-ID',
      name: 'dut',
      netlist: { formalParameters: [], name: 'dut', terminals: [] },
      presentation: {
        compactness: 'normal',
        grid: 10,
        labelSubscriptAfterFirst: false,
        labelSubscriptItalic: false,
        styleProfileId: 'razavi-textbook-v1'
      },
      instances: [],
      routes: [],
      connections: { contacts: [], unrouted: [] },
      nets: [],
      junctions: [],
      annotations: [],
      drafting: { objects: [] },
      connectivityEvidence: [],
      layoutGroups: [],
      constraints: [],
      sourceStatus: 'in-sync',
      revision: 0,
      noConnects: []
    }
  ],
  externalSubcircuitDefinitions: [],
  simulationFolders: [],
  source: { dialect: 'none', entry: null, files: [], sourcePolicy: 'copy' },
  symbolLibrary: { hash: 'razavi-reference-v1', id: 'razavi-symbols', version: '1' },
  structureRevision: 0
}

const text = JSON.stringify(template, null, 2) + '\n'
const project = parseProject(text)
console.log('OK: project name =', project.name)
console.log('OK: top document =', project.topDocumentId)
console.log('OK: documents =', project.documents.length)
console.log('OK: instances =', project.documents[0].instances.length)

// The editor writes the drawing back; the file must survive that round trip.
import { serializeProject } from '@icm/project-protocol'
const written = serializeProject(project)
const reparsed = parseProject(written)
console.log('OK: round trip ->', reparsed.name, '| documents', reparsed.documents.length)
console.log('OK: rewritten length', written.length)
