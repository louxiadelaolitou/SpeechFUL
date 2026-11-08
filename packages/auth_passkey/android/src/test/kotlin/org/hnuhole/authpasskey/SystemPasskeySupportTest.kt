package org.hnuhole.authpasskey

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SystemPasskeySupportTest {
    private val oem = SystemPasskeySupport.Provider("vendor.system.credentials", true, true, true)
    @Test fun optionalCapabilityRequiresNativeApiLockscreenAndBuiltinPublicKeyProvider() {
        assertTrue(SystemPasskeySupport.eligible(34, true, true, listOf(oem)))
        assertFalse(SystemPasskeySupport.eligible(33, true, true, listOf(oem)))
        assertFalse(SystemPasskeySupport.eligible(34, false, true, listOf(oem)))
        assertFalse(SystemPasskeySupport.eligible(34, true, false, listOf(oem)))
        assertFalse(SystemPasskeySupport.eligible(34, true, true, emptyList()))
        assertFalse(SystemPasskeySupport.eligible(34, true, true, listOf(oem.copy(publicKey = false))))
    }
    @Test fun installedThirdPartyAndUnprotectedServicesCannotEnableOptionalBinding() {
        assertFalse(SystemPasskeySupport.eligible(34, true, true, listOf(oem.copy(system = false))))
        assertFalse(SystemPasskeySupport.eligible(34, true, true, listOf(oem.copy(protected = false))))
    }
    @Test fun googleSystemCandidateIsNotOfferedEvenAlongsideAnOemProvider() {
        val google = oem.copy(packageName = "com.google.android.gms")
        assertFalse(SystemPasskeySupport.eligible(34, true, true, listOf(google)))
        assertFalse(SystemPasskeySupport.eligible(34, true, true, listOf(oem, google)))
    }
}
