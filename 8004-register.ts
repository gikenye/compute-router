import { IdentityRegistry } from '@chaoschain/sdk';

const registry = new IdentityRegistry(provider);

// Upload registration file to IPFS first
const agentURI = 'ipfs://QmYourRegistrationFile';

// Register and get agent ID
const tx = await registry.register(agentURI);
const agentId = tx.events.Transfer.returnValues.tokenId;

console.log('Agent registered with ID:', agentId);