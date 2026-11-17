// Product prototype only. No credentials, network calls, device APIs or persistence.
export const fixtures = ['Owner setup', 'Existing devices', 'Not eligible', 'Permission declined', 'Connection failed', 'Peer offline', 'Session declined', 'Membership removed'];
export function initial(scenario = 'Owner setup') {
  if (!fixtures.includes(scenario)) throw Error('unknown scenario');
  return { scenario, screen: scenario === 'Existing devices' || scenario === 'Peer offline' ? 'devices' : 'welcome', workspace: scenario === 'Existing devices' || scenario === 'Peer offline' ? 'Sample workspace' : null, enrolled: scenario === 'Existing devices' || scenario === 'Peer offline', vpn: false, capture: false, control: false, dns: false, traffic: false, session: false, removed: false, denied: false, notice: '' };
}
export function step(s, action) {
  if (action === 'RESET') return initial(s.scenario);
  if (action === 'SIGN_OUT') return { ...initial(s.scenario), enrolled: s.enrolled, screen: 'welcome', notice: 'Signed out in this simulation. Existing device enrollment is separate from sign-in; no real account changed.' };
  if (action === 'MEMBERSHIP_REMOVED') return { ...s, screen: 'blocked', session: false, capture: false, control: false, notice: 'Workspace access was removed. New access is blocked and the simulated session has ended.' };
  if (s.screen === 'blocked') return s;
  if (action === 'NAV_DEVICES' || action === 'NAV_SETTINGS') {
    if (!['devices', 'device', 'settings', 'offline', 'declined', 'ended', 'ready'].includes(s.screen) || s.session) return { ...s, notice: 'Finish or cancel the current step before leaving this screen.' };
    return { ...s, screen: action === 'NAV_DEVICES' ? 'devices' : 'settings', notice: '' };
  }
  if (action === 'NETWORK_LOST' && s.session) return { ...s, screen: 'reconnecting', control: false };
  switch (s.screen) {
    case 'welcome':
      if (action === 'SIGN_IN') return { ...s, screen: 'signin', notice: '' };
      break;
    case 'signin':
      if (action === 'CANCEL') return { ...s, screen: 'welcome' };
      if (action === 'RETURN') return { ...s, screen: s.scenario === 'Not eligible' ? 'blocked' : 'workspace', notice: s.scenario === 'Not eligible' ? 'An identity-provider login alone is not enough. An eligible Multies account and workspace are required.' : '' };
      break;
    case 'workspace':
      if (action === 'CHOOSE') return { ...s, workspace: 'Sample workspace', screen: 'devices' };
      break;
    case 'devices':
      if (action === 'ADD') return { ...s, screen: s.enrolled ? 'another-device' : 'permission', notice: '' };
      if (action === 'DETAIL' && s.enrolled && !s.removed) return { ...s, screen: 'device' };
      if (action === 'SETTINGS') return { ...s, screen: 'settings' };
      break;
    case 'another-device':
      if (action === 'BACK') return { ...s, screen: 'devices' };
      break;
    case 'permission':
      if (action === 'CANCEL') return { ...s, screen: 'devices' };
      if (action === 'DECLINE' || action === 'ALLOW' && s.scenario === 'Permission declined' && !s.denied) return { ...s, denied: true, vpn: false, notice: 'Permission was not granted. Nothing connected. You can try again or leave setup.' };
      if (action === 'ALLOW') return { ...s, vpn: true, screen: 'connecting', notice: '' };
      break;
    case 'connecting':
      if (action === 'CANCEL') return { ...s, vpn: false, screen: 'devices' };
      if (action === 'CHECK') return { ...s, screen: 'checks', dns: s.scenario !== 'Connection failed', traffic: false };
      break;
    case 'checks':
      if (action === 'VERIFY' && s.dns) return { ...s, traffic: true, enrolled: true, screen: 'ready' };
      if (action === 'VERIFY' && !s.dns) return { ...s, screen: 'failed', vpn: false, notice: 'Private name lookup failed. Setup stopped instead of claiming Connected. Ordinary internet must remain available.' };
      if (action === 'CANCEL') return { ...s, vpn: false, screen: 'devices' };
      break;
    case 'failed':
      if (action === 'RETRY') return { ...s, screen: 'permission', notice: '' };
      if (action === 'CANCEL') return { ...s, screen: 'devices' };
      break;
    case 'ready':
      if (action === 'DONE') return { ...s, screen: 'devices' };
      break;
    case 'device':
      if (action === 'BACK') return { ...s, screen: 'devices' };
      if (action === 'REMOVE') return { ...s, screen: 'remove' };
      if (action === 'CONNECT') return { ...s, screen: s.scenario === 'Peer offline' ? 'offline' : 'request' };
      break;
    case 'request':
      if (action === 'CANCEL') return { ...s, screen: 'device' };
      if (action === 'REQUEST') return { ...s, screen: 'waiting' };
      break;
    case 'waiting':
      if (action === 'CANCEL') return { ...s, screen: 'device', notice: 'Request cancelled. A late approval cannot start a session.' };
      if (action === 'HOST_REPLY') return { ...s, screen: s.scenario === 'Session declined' ? 'declined' : 'host-consent' };
      break;
    case 'host-consent':
      if (action === 'DECLINE') return { ...s, screen: 'declined' };
      if (action === 'VIEW') return { ...s, screen: 'active', session: true, capture: true, control: false };
      break;
    case 'active':
      if (action === 'CONTROL_REQUEST') return { ...s, screen: 'control-consent' };
      if (action === 'NETWORK_LOST') return { ...s, screen: 'reconnecting', control: false };
      if (action === 'END') return { ...s, screen: 'ended', session: false, capture: false, control: false };
      break;
    case 'control-consent':
      if (action === 'DECLINE') return { ...s, screen: 'active', control: false };
      if (action === 'HOST_ALLOW') return { ...s, screen: 'active', control: true };
      if (action === 'END') return { ...s, screen: 'ended', session: false, capture: false, control: false };
      break;
    case 'reconnecting':
      if (action === 'RECOVER') return { ...s, screen: 'active', control: false, notice: 'View restored after revalidation. Control needs fresh host approval.' };
      if (action === 'TIMEOUT') return { ...s, screen: 'offline', session: false, capture: false, control: false };
      if (action === 'END') return { ...s, screen: 'ended', session: false, capture: false, control: false };
      break;
    case 'remove':
      if (action === 'CANCEL') return { ...s, screen: 'device' };
      if (action === 'CONFIRM_REMOVE') return { ...s, screen: 'devices', removed: true, session: false, capture: false, control: false, notice: 'Sample laptop removed. Re-enrollment requires new authorization; old keys cannot restore it.' };
      break;
    case 'settings':
      if (action === 'BACK') return { ...s, screen: 'devices' };
      if (action === 'DISCONNECT') return { ...s, vpn: false, session: false, capture: false, control: false, notice: 'Disconnected in the simulation. Device ownership is kept; remote access is stopped.' };
      break;
    case 'offline': case 'declined': case 'ended':
      if (action === 'BACK') return { ...s, screen: 'devices' };
      break;
  }
  return s; // Unknown, stale and out-of-order events never grant a capability.
}
