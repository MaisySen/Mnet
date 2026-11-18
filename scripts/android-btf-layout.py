"""Minimal BTF reader for verified byte offsets, no guessed kernel layouts."""
import struct
class BTF:
    def __init__(self,data):
        magic,version,flags,hdr,off,length,so,sl=struct.unpack_from('<HBBIIIII',data)
        if magic!=0xeb9f or version!=1:raise ValueError('unsupported BTF')
        strings=data[hdr+so:hdr+so+sl]
        def name(n):return strings[n:strings.index(0,n)].decode()
        self.types=[None];at=hdr+off;end=at+length
        while at<end:
            n,info,size=struct.unpack_from('<III',data,at);at+=12
            kind=(info>>24)&31;vlen=info&65535;kflag=info>>31
            t={'name':name(n),'kind':kind,'size':size,'members':[]}
            if kind in (4,5):
                for _ in range(vlen):
                    mn,mt,mo=struct.unpack_from('<III',data,at);at+=12
                    bits=mo&0xffffff if kflag else mo
                    if bits%8:continue
                    t['members'].append((name(mn),mt,bits//8))
            elif kind==13:
                t['params']=[]
                for _ in range(vlen):
                    pn,pt=struct.unpack_from('<II',data,at);at+=8
                    t['params'].append((name(pn),pt))
            elif kind==6:
                t['values']={}
                for _ in range(vlen):
                    en,ev=struct.unpack_from('<Ii',data,at);at+=8
                    t['values'][name(en)]=ev
            else:
                extra={1:4,2:0,3:12,6:vlen*8,7:0,8:0,9:0,10:0,11:0,12:0,13:vlen*8,14:4,15:vlen*12,16:0,17:4,18:0,19:vlen*12}.get(kind)
                if extra is None:raise ValueError('BTF kind')
                at+=extra
            self.types.append(t)
        if at!=end:raise ValueError('BTF boundary')
    def unwrap(self,i):
        for _ in range(32):
            t=self.types[i]
            if t['kind'] not in (8,9,10,11,18):return i
            i=t['size']
        raise ValueError('BTF cycle')
    def member(self,i,field,depth=0):
        if depth>20:raise ValueError('BTF depth')
        t=self.types[self.unwrap(i)]
        for n,mt,off in t['members']:
            if n==field:return mt,off
        for n,mt,off in t['members']:
            if n=='':
                try:
                    found,child=self.member(mt,field,depth+1);return found,off+child
                except KeyError:pass
        raise KeyError(field)
    def offset(self,typename,path):
        candidates=[i for i,t in enumerate(self.types) if t and t['name']==typename and t['kind'] in (4,5) and t['members']]
        if len(candidates)!=1:raise ValueError('ambiguous BTF type')
        i=candidates[0];total=0
        for field in path.split('.'):
            i,off=self.member(i,field);total+=off
        return total

    def function_parameters(self,name):
        found=[t for t in self.types if t and t['kind']==12 and t['name']==name]
        if len(found)!=1:raise ValueError('ambiguous BTF function')
        proto=self.types[found[0]['size']]
        return proto.get('params',[])
    def enum_value(self,enum,name):
        found=[t for t in self.types if t and t['kind']==6 and t['name']==enum and name in t.get('values',{})]
        if len(found)!=1:raise ValueError('ambiguous BTF enum')
        return found[0]['values'][name]
